package mounter

import (
	"context"
	"errors"
	"fmt"
	xsync "golang.org/x/sync/errgroup"
	"sync"
	"time"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/aloknerurkar/bee-fs/pkg/fuse"
	"github.com/aloknerurkar/bee-fs/pkg/store"
	"github.com/ethersphere/bee/pkg/swarm"
	logger "github.com/ipfs/go-log/v2"
	"github.com/robfig/cron/v3"
)

var log = logger.Logger("mounter")

type BeeFsMounter interface {
	Mount(context.Context, string, ...MountOption) (MountInfo, error)
	Unmount(context.Context, string) error
	IsActive(string) bool
	Get(string) (MountInfo, error)
	List() []MountInfo
	Snapshot(context.Context, string) (swarm.Address, error)
}

type SnapshotInfo struct {
	Name      string
	Timestamp time.Time
	Reference swarm.Address
}

type MountInfo struct {
	Path         string
	Active       bool
	SnapshotSpec string
	ID           cron.EntryID
	KeepCount    int
	Encryption   bool
	BeeHostAddr  string
	Snapshots    []SnapshotInfo
	LastStatus   string
	LastRun      time.Time
	NextRun      time.Time
}

type MountOption interface {
	apply(*mount)
}

type mountOptionFunc func(*mount)

func (f mountOptionFunc) apply(m *mount) {
	f(m)
}

func WithSnapshotSpec(spec string) MountOption {
	return mountOptionFunc(func(m *mount) { m.info.SnapshotSpec = spec })
}

func WithKeepCount(keep int) MountOption {
	return mountOptionFunc(func(m *mount) { m.info.KeepCount = keep })
}

func WithEncryption() MountOption {
	return mountOptionFunc(func(m *mount) { m.info.Encryption = true })
}

func WithStorage(st store.PutGetter) MountOption {
	return mountOptionFunc(func(m *mount) {
		m.beeStore = st
		m.info.BeeHostAddr = st.Info()
	})
}

type beeFsMounter struct {
	mnts          sync.Map
	snapScheduler *cron.Cron
}

func New() BeeFsMounter {
	return &beeFsMounter{
		snapScheduler: cron.New(),
	}
}

type mount struct {
	fsImpl    *fs.BeeFs
	host      *fuse.FileSystemHost
	info      MountInfo
	beeStore  store.PutGetter
	mntWorker xsync.Group
	snapLock  sync.Mutex
}

func (m *mount) Run() {
	m.snapLock.Lock()
	defer m.snapLock.Unlock()

	if !m.info.Active {
		log.Warnf("mount not active")
		m.info.LastStatus = "mount not active"
		return
	}

	err := m.snapshot()
	if err != nil {
		if errors.Is(err, ErrNoDiff) {
			m.info.LastStatus = "no changes since last snapshot"
			return
		}
		m.info.LastStatus = "failed reason: " + err.Error()
		return
	}

	m.info.LastStatus = "successful"
}

var ErrNoDiff = errors.New("no diff")

func (m *mount) snapshot() error {
	ref, err := m.fsImpl.Snapshot()
	if err != nil {
		return err
	}

	if len(m.info.Snapshots) > 0 && m.info.Snapshots[len(m.info.Snapshots)-1].Reference.Equal(ref) {
		return errors.New("no diff")
	}

	m.info.Snapshots = append(m.info.Snapshots, SnapshotInfo{
		Reference: ref,
		Timestamp: time.Now(),
		Name:      fmt.Sprintf("snap_%d", len(m.info.Snapshots)),
	})

	if len(m.info.Snapshots) > m.info.KeepCount {
		m.info.Snapshots = m.info.Snapshots[1:]
	}

	return nil
}

func (b *beeFsMounter) Mount(
	ctx context.Context,
	mntDir string,
	opts ...MountOption,
) (info MountInfo, err error) {

	mnt := &mount{
		info: MountInfo{
			Path: mntDir,
		},
	}
	_, loaded := b.mnts.LoadOrStore(mntDir, mnt)
	if loaded {
		return info, errors.New("already mounted")
	}
	for _, opt := range opts {
		opt.apply(mnt)
	}
	if mnt.info.SnapshotSpec != "" {
		schedule, err := cron.ParseStandard(mnt.info.SnapshotSpec)
		if err != nil {
			return info, err
		}
		mnt.info.ID = b.snapScheduler.Schedule(schedule, mnt)
	}
	if mnt.beeStore == nil {
		mnt.beeStore = store.NewAPIStore("localhost", 1633, false)
		mnt.info.BeeHostAddr = "localhost:1633"
	}
	mnt.fsImpl, err = fs.New(mnt.beeStore)
	if err != nil {
		return info, err
	}
	mnt.host = fuse.NewFileSystemHost(mnt.fsImpl)
	mnt.host.SetCapReaddirPlus(true)

	mounted := make(chan struct{})
	mnt.mntWorker.Go(func() error {
		close(mounted)
		mnt.info.Active = true
		mnt.host.Mount(mntDir, nil)
		mnt.info.Active = false
		return nil
	})

	select {
	case <-mounted:
	case <-ctx.Done():
		mnt.host.Unmount()
		return info, ctx.Err()
	}

	return mnt.info, nil
}

func (b *beeFsMounter) Unmount(ctx context.Context, mntDir string) (err error) {
	val, ok := b.mnts.Load(mntDir)
	if !ok {
		return errors.New("mount not found")
	}
	mnt := val.(*mount)
	if !mnt.info.Active {
		return errors.New("mount not active")
	}
	mnt.host.Unmount()
	stopped := make(chan struct{})
	go func() {
		mnt.mntWorker.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (b *beeFsMounter) IsActive(path string) bool {
	val, ok := b.mnts.Load(path)
	if !ok {
		return false
	}
	return val.(*mount).info.Active
}

func (b *beeFsMounter) Get(path string) (retMnt MountInfo, err error) {
	val, ok := b.mnts.Load(path)
	if !ok {
		return retMnt, errors.New("not found")
	}
	sentry := b.snapScheduler.Entry(val.(*mount).info.ID)
	val.(*mount).info.LastRun = sentry.Prev
	val.(*mount).info.NextRun = sentry.Next
	return val.(*mount).info, nil
}

func (b *beeFsMounter) List() (retMnts []MountInfo) {
	b.mnts.Range(func(_, v interface{}) bool {
		sentry := b.snapScheduler.Entry(v.(*mount).info.ID)
		v.(*mount).info.LastRun = sentry.Prev
		v.(*mount).info.NextRun = sentry.Next
		retMnts = append(retMnts, v.(*mount).info)
		return true
	})
	return retMnts
}

func (b *beeFsMounter) Snapshot(ctx context.Context, mntDir string) (swarm.Address, error) {
	val, ok := b.mnts.Load(mntDir)
	if !ok {
		return swarm.ZeroAddress, errors.New("mount not found")
	}
	mnt := val.(*mount)
	mnt.Run()
	if mnt.info.LastStatus == "successful" ||
		mnt.info.LastStatus == "no changes since last snapshot" {
		return mnt.info.Snapshots[len(mnt.info.Snapshots)-1].Reference, nil
	}
	return swarm.ZeroAddress, errors.New(mnt.info.LastStatus)
}
