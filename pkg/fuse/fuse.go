package fs

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/billziss-gh/cgofuse/fuse"
	bf "github.com/aloknerurkar/bee-fs/pkg/file"
	"github.com/aloknerurkar/bee-fs/pkg/store"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/manifest/mantaray"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	logger "github.com/ipfs/go-log/v2"
)

const (
	MetadataKey = "metadata"
)

var log = logger.Logger("fuse/beeFs")

func trace(start time.Time, errc *int, vals ...interface{}) {
	pc, _, _, ok := runtime.Caller(1)
	name := "<UNKNOWN>"
	if ok {
		fn := runtime.FuncForPC(pc)
		name = fn.Name()
	}
	args := "("
	for idx, v := range vals {
		switch v.(type) {
		case string:
			args += v.(string)
		case int:
			args += strconv.Itoa(v.(int))
		case int64:
			args += strconv.FormatInt(v.(int64), 10)
		case uint64:
			args += strconv.FormatUint(v.(uint64), 10)
		}
		if idx != len(vals)-1 {
			args += ", "
		}
	}
	args += ")"
	log.Debugf("%s took %s args: %s result: %d", name,
		time.Since(start).String(), args, *errc)
}

type FsNode interface {
	Reference() swarm.Address
	IsDir() bool
	Metadata() string
}

func split(path string) []string {
	return strings.Split(path, "/")
}

type fsNode struct {
	stat    fuse.Stat_t
	xatr    map[string][]byte
	chld    map[string]*fsNode
	data    *bf.BeeFile
	opencnt int
}

func (f *fsNode) Reference() swarm.Address {
	if !f.IsDir() {
		ref, err := f.data.Close()
		if err == nil {
			return ref
		}
	}
	return swarm.ZeroAddress
}

func (f *fsNode) IsDir() bool {
	if len(f.chld) > 0 {
		return true
	}
	return false
}

func (f *fsNode) Metadata() string {
	return ""
}

type BeeFs struct {
	fuse.FileSystemBase
	lock      sync.Mutex
	ino       uint64
	root      *fsNode
	openmap   map[uint64]*fsNode
	rm        *mantaray.Node
	ls        file.LoadSaver
	store     store.PutGetter
	reference swarm.Address
	pin       bool
	encrypt   bool
}

type Option func(*BeeFs)

func WithReference(ref swarm.Address) Option {
	return func(r *BeeFs) {
		r.reference = ref
	}
}

func WithPin(val bool) Option {
	return func(r *BeeFs) {
		r.pin = val
	}
}

func WithEncryption(val bool) Option {
	return func(r *BeeFs) {
		r.encrypt = val
	}
}

func New(st store.PutGetter, opts ...Option) (*BeeFs, error) {
	b := &BeeFs{
		reference: swarm.ZeroAddress,
	}
	for _, opt := range opts {
		opt(b)
	}
	b.store = st
	mode := storage.ModePutUpload
	if b.pin {
		mode = storage.ModePutUploadPin
	}
	b.ls = loadsave.New(b.store, mode, b.encrypt)
	if b.reference.Equal(swarm.ZeroAddress) {
		m := mantaray.New()
		b.rm = m
		b.ino++
		b.root = b.newNode(0, b.ino, fuse.S_IFDIR|00777, 0, 0)
		b.openmap = map[uint64]*fsNode{}
	} else {
		m := mantaray.NewNodeRef(b.reference.Bytes())
		b.rm = m
	}
	return b, nil
}

func (b *BeeFs) Mknod(path string, mode uint32, dev uint64) (errc int) {
	defer trace(time.Now(), &errc, path, mode, dev)
	defer b.synchronize()()
	return b.makeNode(path, mode, dev, nil)
}

func (b *BeeFs) Mkdir(path string, mode uint32) (errc int) {
	defer trace(time.Now(), &errc, path, mode)
	defer b.synchronize()()
	return b.makeNode(path, fuse.S_IFDIR|(mode&07777), 0, nil)
}

func (b *BeeFs) Unlink(path string) (errc int) {
	defer trace(time.Now(), &errc, path)
	defer b.synchronize()()
	return b.removeNode(path, false)
}

func (b *BeeFs) Rmdir(path string) (errc int) {
	defer trace(time.Now(), &errc, path)
	defer b.synchronize()()
	return b.removeNode(path, true)
}

func (b *BeeFs) Link(oldpath string, newpath string) (errc int) {
	defer trace(time.Now(), &errc, oldpath, newpath)
	defer b.synchronize()()
	_, _, oldnode := b.lookupNode(oldpath, nil)
	if nil == oldnode {
		return -fuse.ENOENT
	}
	newprnt, newname, newnode := b.lookupNode(newpath, nil)
	if nil == newprnt {
		return -fuse.ENOENT
	}
	if nil != newnode {
		return -fuse.EEXIST
	}
	oldnode.stat.Nlink++
	newprnt.chld[newname] = oldnode
	tmsp := fuse.Now()
	oldnode.stat.Ctim = tmsp
	newprnt.stat.Ctim = tmsp
	newprnt.stat.Mtim = tmsp
	return 0
}

func (b *BeeFs) Symlink(target string, newpath string) (errc int) {
	defer trace(time.Now(), &errc, target, newpath)
	defer b.synchronize()()
	return b.makeNode(newpath, fuse.S_IFLNK|00777, 0, []byte(target))
}

func (b *BeeFs) Readlink(path string) (errc int, target string) {
	defer trace(time.Now(), &errc, path)
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT, ""
	}
	if fuse.S_IFLNK != node.stat.Mode&fuse.S_IFMT {
		return -fuse.EINVAL, ""
	}
	linkBuf := make([]byte, 1024)
	n, err := node.data.ReadAt(linkBuf, 0)
	if err != nil {
		return -fuse.EIO, ""
	}
	return 0, string(linkBuf[:n])
}

func (b *BeeFs) Rename(oldpath string, newpath string) (errc int) {
	defer trace(time.Now(), &errc, oldpath, newpath)
	defer b.synchronize()()
	oldprnt, oldname, oldnode := b.lookupNode(oldpath, nil)
	if nil == oldnode {
		return -fuse.ENOENT
	}
	newprnt, newname, newnode := b.lookupNode(newpath, oldnode)
	if nil == newprnt {
		return -fuse.ENOENT
	}
	if "" == newname {
		// guard against directory loop creation
		return -fuse.EINVAL
	}
	if oldprnt == newprnt && oldname == newname {
		return 0
	}
	if nil != newnode {
		errc = b.removeNode(newpath, fuse.S_IFDIR == oldnode.stat.Mode&fuse.S_IFMT)
		if 0 != errc {
			return errc
		}
	}
	delete(oldprnt.chld, oldname)
	newprnt.chld[newname] = oldnode
	return 0
}

func (b *BeeFs) Chmod(path string, mode uint32) (errc int) {
	defer trace(time.Now(), &errc, path, mode)
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	node.stat.Mode = (node.stat.Mode & fuse.S_IFMT) | mode&07777
	node.stat.Ctim = fuse.Now()
	return 0
}

func (b *BeeFs) Chown(path string, uid uint32, gid uint32) (errc int) {
	defer trace(time.Now(), &errc, path, uid, gid)
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	if ^uint32(0) != uid {
		node.stat.Uid = uid
	}
	if ^uint32(0) != gid {
		node.stat.Gid = gid
	}
	node.stat.Ctim = fuse.Now()
	return 0
}

func (b *BeeFs) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	defer trace(time.Now(), &errc, path)
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	node.stat.Ctim = fuse.Now()
	if nil == tmsp {
		tmsp0 := node.stat.Ctim
		tmsa := [2]fuse.Timespec{tmsp0, tmsp0}
		tmsp = tmsa[:]
	}
	node.stat.Atim = tmsp[0]
	node.stat.Mtim = tmsp[1]
	return 0
}

func (b *BeeFs) Open(path string, flags int) (errc int, fh uint64) {
	defer trace(time.Now(), &errc, path, flags)
	defer b.synchronize()()
	return b.openNode(path, false)
}

func (b *BeeFs) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	defer trace(time.Now(), &errc, path, fh)
	defer b.synchronize()()
	node := b.getNode(path, fh)
	if nil == node {
		return -fuse.ENOENT
	}
	*stat = node.stat
	return 0
}

func (b *BeeFs) Truncate(path string, size int64, fh uint64) (errc int) {
	defer trace(time.Now(), &errc, path, size, fh)
	defer b.synchronize()()
	node := b.getNode(path, fh)
	if nil == node {
		return -fuse.ENOENT
	}
	err := node.data.Truncate(size)
	if err != nil {
		return -fuse.EIO
	}
	node.stat.Size = size
	tmsp := fuse.Now()
	node.stat.Ctim = tmsp
	node.stat.Mtim = tmsp
	return 0
}

func (b *BeeFs) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	defer trace(time.Now(), &n, path, ofst, fh)
	defer b.synchronize()()
	node := b.getNode(path, fh)
	if nil == node {
		return -fuse.ENOENT
	}
	var err error
	n, err = node.data.ReadAt(buff, ofst)
	if err != nil {
		return -fuse.EIO
	}
	node.stat.Atim = fuse.Now()
	return n
}

func (b *BeeFs) Write(path string, buff []byte, ofst int64, fh uint64) (n int) {
	defer trace(time.Now(), &n, path, ofst, fh)
	defer b.synchronize()()
	node := b.getNode(path, fh)
	if nil == node {
		return -fuse.ENOENT
	}
	endofst := ofst + int64(len(buff))
	if endofst > node.stat.Size {
		node.stat.Size = endofst
	}
	var err error
	n, err = node.data.WriteAt(buff, ofst)
	if err != nil {
		return -fuse.EIO
	}
	tmsp := fuse.Now()
	node.stat.Ctim = tmsp
	node.stat.Mtim = tmsp
	return n
}

func (b *BeeFs) Release(path string, fh uint64) (errc int) {
	defer trace(time.Now(), &errc, path, fh)
	defer b.synchronize()()
	return b.closeNode(fh)
}

func (b *BeeFs) Opendir(path string) (errc int, fh uint64) {
	defer trace(time.Now(), &errc, path)
	defer b.synchronize()()
	return b.openNode(path, true)
}

func (b *BeeFs) Readdir(
	path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64,
) (errc int) {
	defer trace(time.Now(), &errc, path, ofst, fh)
	defer b.synchronize()()
	node := b.openmap[fh]
	fill(".", &node.stat, 0)
	fill("..", nil, 0)
	for name, chld := range node.chld {
		if !fill(name, &chld.stat, 0) {
			break
		}
	}
	return 0
}

func (b *BeeFs) Releasedir(path string, fh uint64) (errc int) {
	defer trace(time.Now(), &errc, path, fh)
	defer b.synchronize()()
	return b.closeNode(fh)
}

func (b *BeeFs) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	defer trace(time.Now(), &errc, path, name, flags)
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	if "com.apple.ResourceFork" == name {
		return -fuse.ENOTSUP
	}
	if fuse.XATTR_CREATE == flags {
		if _, ok := node.xatr[name]; ok {
			return -fuse.EEXIST
		}
	} else if fuse.XATTR_REPLACE == flags {
		if _, ok := node.xatr[name]; !ok {
			return -fuse.ENOATTR
		}
	}
	xatr := make([]byte, len(value))
	copy(xatr, value)
	if nil == node.xatr {
		node.xatr = map[string][]byte{}
	}
	node.xatr[name] = xatr
	return 0
}

func (b *BeeFs) Getxattr(path string, name string) (errc int, xatr []byte) {
	defer trace(time.Now(), &errc, path, name)
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT, nil
	}
	if "com.apple.ResourceFork" == name {
		return -fuse.ENOTSUP, nil
	}
	xatr, ok := node.xatr[name]
	if !ok {
		return -fuse.ENOATTR, nil
	}
	return 0, xatr
}

func (b *BeeFs) Removexattr(path string, name string) (errc int) {
	defer trace(time.Now(), &errc, path, name)
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	if "com.apple.ResourceFork" == name {
		return -fuse.ENOTSUP
	}
	if _, ok := node.xatr[name]; !ok {
		return -fuse.ENOATTR
	}
	delete(node.xatr, name)
	return 0
}

func (b *BeeFs) Listxattr(path string, fill func(name string) bool) (errc int) {
	defer trace(time.Now(), &errc, path)
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	for name := range node.xatr {
		if !fill(name) {
			return -fuse.ERANGE
		}
	}
	return 0
}

func (b *BeeFs) Chflags(path string, flags uint32) (errc int) {
	defer trace(time.Now(), &errc, path, flags)
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	node.stat.Flags = flags
	node.stat.Ctim = fuse.Now()
	return 0
}

func (b *BeeFs) Setcrtime(path string, tmsp fuse.Timespec) (errc int) {
	defer trace(time.Now(), &errc, path, tmsp.Time().String())
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	node.stat.Birthtim = tmsp
	node.stat.Ctim = fuse.Now()
	return 0
}

func (b *BeeFs) Setchgtime(path string, tmsp fuse.Timespec) (errc int) {
	defer trace(time.Now(), &errc, path, tmsp.Time().String())
	defer b.synchronize()()
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	node.stat.Ctim = tmsp
	return 0
}

type WalkFunc func(path string, nd FsNode) (err error, stop bool)

func (b *BeeFs) Walk(ctx context.Context, walker WalkFunc) error {
	var nodesToWalk nodeQueue
	nodesToWalk.push(string(os.PathSeparator), b.root)
	for p, currentPath := nodesToWalk.pop(); p != nil; p, currentPath = nodesToWalk.pop() {
		err, stop := walker(currentPath, p)
		if stop {
			return err
		}
		if p.IsDir() {
			for k, v := range p.chld {
				nodesToWalk.push(filepath.Join(currentPath, k), v)
			}
		}
	}
	return nil
}

func (b *BeeFs) Snapshot() (swarm.Address, error) {
	ctx, _ := context.WithTimeout(context.Background(), time.Minute*15)
	m := mantaray.New()
	err := b.Walk(ctx, func(path string, nd FsNode) (error, bool) {
		err := m.Add(
			ctx,
			[]byte(path),
			nd.Reference().Bytes(),
			map[string]string{MetadataKey: nd.Metadata()},
			b.ls,
		)
		if err != nil {
			return err, true
		}
		return nil, false
	})
	if err != nil {
		return swarm.ZeroAddress, err
	}

	err = m.Save(context.Background(), b.ls)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(m.Reference()), nil
}

type nodeWithPath struct {
	nd   *fsNode
	path string
}

type nodeQueue struct {
	items []nodeWithPath
}

func (n nodeQueue) push(path string, v *fsNode) {
	if n.items == nil {
		n.items = make([]nodeWithPath, 0)
	}
	n.items = append(n.items, nodeWithPath{v, path})
}

func (n nodeQueue) pop() (*fsNode, string) {
	var res nodeWithPath
	if len(n.items) == 0 {
		return nil, ""
	}
	res, n.items = n.items[0], n.items[1:]
	return res.nd, res.path
}

func (b *BeeFs) newNode(dev uint64, ino uint64, mode uint32, uid uint32, gid uint32) *fsNode {
	tmsp := fuse.Now()
	f := fsNode{
		fuse.Stat_t{
			Dev:      dev,
			Ino:      ino,
			Mode:     mode,
			Nlink:    1,
			Uid:      uid,
			Gid:      gid,
			Atim:     tmsp,
			Mtim:     tmsp,
			Ctim:     tmsp,
			Birthtim: tmsp,
			Flags:    0,
		},
		nil,
		nil,
		nil,
		0}
	if fuse.S_IFDIR == f.stat.Mode&fuse.S_IFMT {
		f.chld = map[string]*fsNode{}
	} else {
		f.data = bf.New(swarm.ZeroAddress, b.store, b.encrypt)
	}
	return &f
}

func (b *BeeFs) lookupNode(path string, ancestor *fsNode) (prnt *fsNode, name string, node *fsNode) {
	prnt = b.root
	name = ""
	node = b.root
	for _, c := range split(path) {
		if "" != c {
			if 255 < len(c) {
				panic(fuse.Error(-fuse.ENAMETOOLONG))
			}
			prnt, name = node, c
			if node == nil {
				return
			}
			node = node.chld[c]
			if nil != ancestor && node == ancestor {
				name = "" // special case loop condition
				return
			}
		}
	}
	return
}

func (b *BeeFs) makeNode(path string, mode uint32, dev uint64, data []byte) int {
	prnt, name, node := b.lookupNode(path, nil)
	if nil == prnt {
		return -fuse.ENOENT
	}
	if nil != node {
		return -fuse.EEXIST
	}
	b.ino++
	uid, gid, _ := fuse.Getcontext()
	node = b.newNode(dev, b.ino, mode, uid, gid)
	if nil != data {
		node.data = bf.New(swarm.ZeroAddress, b.store, b.encrypt)
		n, err := node.data.Write(data)
		if err != nil {
			return -fuse.EIO
		}
		node.stat.Size = int64(n)
	}
	prnt.chld[name] = node
	prnt.stat.Ctim = node.stat.Ctim
	prnt.stat.Mtim = node.stat.Ctim
	return 0
}

func (b *BeeFs) removeNode(path string, dir bool) int {
	prnt, name, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	if !dir && fuse.S_IFDIR == node.stat.Mode&fuse.S_IFMT {
		return -fuse.EISDIR
	}
	if dir && fuse.S_IFDIR != node.stat.Mode&fuse.S_IFMT {
		return -fuse.ENOTDIR
	}
	if 0 < len(node.chld) {
		return -fuse.ENOTEMPTY
	}
	node.stat.Nlink--
	delete(prnt.chld, name)
	tmsp := fuse.Now()
	node.stat.Ctim = tmsp
	prnt.stat.Ctim = tmsp
	prnt.stat.Mtim = tmsp
	return 0
}

func (b *BeeFs) openNode(path string, dir bool) (int, uint64) {
	_, _, node := b.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT, ^uint64(0)
	}
	if !dir && fuse.S_IFDIR == node.stat.Mode&fuse.S_IFMT {
		return -fuse.EISDIR, ^uint64(0)
	}
	if dir && fuse.S_IFDIR != node.stat.Mode&fuse.S_IFMT {
		return -fuse.ENOTDIR, ^uint64(0)
	}
	node.opencnt++
	if 1 == node.opencnt {
		b.openmap[node.stat.Ino] = node
	}
	return 0, node.stat.Ino
}

func (b *BeeFs) closeNode(fh uint64) int {
	node := b.openmap[fh]
	node.opencnt--
	if 0 == node.opencnt {
		if node.data != nil {
			err := node.data.Sync()
			if err != nil {
				return -fuse.EIO
			}
		}
		delete(b.openmap, node.stat.Ino)
	}
	return 0
}

func (b *BeeFs) getNode(path string, fh uint64) *fsNode {
	if ^uint64(0) == fh {
		_, _, node := b.lookupNode(path, nil)
		return node
	} else {
		return b.openmap[fh]
	}
}

func (b *BeeFs) synchronize() func() {
	b.lock.Lock()
	return func() {
		b.lock.Unlock()
	}
}

var _ fuse.FileSystemChflags = (*BeeFs)(nil)
var _ fuse.FileSystemSetcrtime = (*BeeFs)(nil)
var _ fuse.FileSystemSetchgtime = (*BeeFs)(nil)
