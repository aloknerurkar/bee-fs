package mounter

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/errgroup"
	"io"
	"sync"
	"time"

	"github.com/aloknerurkar/bee-fs/pkg/fuse"
	"github.com/aloknerurkar/bee-fs/pkg/store"
	"github.com/billziss-gh/cgofuse/fuse"
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
	SnapshotInfo(context.Context, string) []SnapshotInfo
	io.Closer
}

type SnapshotInfo struct {
	Name      string
	Timestamp time.Time
	Reference swarm.Address
	Tag       string
	Stats     store.BackupStat
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
		if info, ok := st.(store.StoreInfo); ok {
			m.info.BeeHostAddr = info.Info()
		}
	})
}

func WithReference(ref swarm.Address) MountOption {
	return mountOptionFunc(func(m *mount) { m.reference = ref })
}

type beeFsMounter struct {
	mnts          sync.Map
	snapScheduler *cron.Cron
}

func New() BeeFsMounter {
	c := cron.New()
	c.Start()
	return &beeFsMounter{
		snapScheduler: c,
	}
}

type mount struct {
	fsImpl    *fs.BeeFs
	host      *fuse.FileSystemHost
	info      MountInfo
	beeStore  store.PutGetter
	mntWorker errgroup.Group
	snapLock  sync.Mutex
	reference swarm.Address
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
		return ErrNoDiff
	}

	snap := SnapshotInfo{
		Reference: ref,
		Timestamp: time.Now(),
		Name:      fmt.Sprintf("snap_%d", time.Now().Unix()),
	}

	if bkpr, ok := m.beeStore.(store.Backuper); ok {
		stat, err := bkpr.Backup(context.Background(), ref)
		if err != nil {
			return fmt.Errorf("failed to backup, reason: %w", err)
		}
		snap.Tag = stat
	}

	m.info.Snapshots = append(m.info.Snapshots, snap)

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
	m, loaded := b.mnts.LoadOrStore(mntDir, mnt)
	if loaded {
		mnt = m.(*mount)
		if mnt.info.Active {
			return info, errors.New("already mounted")
		}
	}
	for _, opt := range opts {
		opt.apply(mnt)
	}
	if mnt.beeStore == nil {
		return info, errors.New("storage not configured")
	}
	if mnt.info.SnapshotSpec != "" {
		schedule, err := cron.ParseStandard(mnt.info.SnapshotSpec)
		if err != nil {
			return info, err
		}
		mnt.info.ID = b.snapScheduler.Schedule(schedule, mnt)
	}
	fuseOpts := []fs.Option{fs.WithEncryption(mnt.info.Encryption)}
	if !mnt.reference.Equal(swarm.ZeroAddress) {
		fuseOpts = append(fuseOpts, fs.WithReference(mnt.reference))
	}
	mnt.fsImpl, err = fs.New(mnt.beeStore, fuseOpts...)
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

	mnt.snapLock.Lock()
	defer mnt.snapLock.Unlock()

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

	if closer, ok := mnt.beeStore.(io.Closer); ok {
		return closer.Close()
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
	mnt := val.(*mount)
	ctx := context.Background()
	if backupSt, ok := mnt.beeStore.(store.Backuper); ok {
		for i, v := range mnt.info.Snapshots {
			mnt.info.Snapshots[i].Stats = backupSt.Status(ctx, v.Tag)
		}
	}
	sentry := b.snapScheduler.Entry(mnt.info.ID)
	mnt.info.LastRun = sentry.Prev
	mnt.info.NextRun = sentry.Next
	return mnt.info, nil
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

func (b *beeFsMounter) SnapshotInfo(ctx context.Context, mntDir string) (snapInfos []SnapshotInfo) {
	val, ok := b.mnts.Load(mntDir)
	if !ok {
		return
	}
	mnt := val.(*mount)
	snapInfos = mnt.info.Snapshots
	if backupSt, ok := mnt.beeStore.(store.Backuper); ok {
		for i, v := range snapInfos {
			snapInfos[i].Stats = backupSt.Status(ctx, v.Tag)
		}
	}
	return snapInfos
}

func (b *beeFsMounter) Close() error {
	b.snapScheduler.Stop()
	ctx, _ := context.WithTimeout(context.Background(), time.Minute*5)
	b.mnts.Range(func(k, v interface{}) bool {
		b.Unmount(ctx, k.(string))
		return true
	})
	return nil
}
