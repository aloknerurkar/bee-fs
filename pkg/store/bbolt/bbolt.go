package boltstore

import (
	"context"
	"errors"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"path/filepath"

	"github.com/aloknerurkar/bee-fs/pkg/store"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
)

var defaultBucket = []byte("bucket")
var defaultFilename = "beefs.db"

type boltStore struct {
	path        string
	db          *bolt.DB
	traverser   traversal.Traverser
	backupStore store.BackupStore
}

func NewBoltStore(path string, backup store.BackupStore) (store.PutGetter, error) {
	dbpath := filepath.Join(path, defaultFilename)
	db, err := bolt.Open(dbpath, 0666, nil)
	if err != nil {
		return nil, err
	}
	err = db.Update(func(t *bolt.Tx) error {
		_, err := t.CreateBucketIfNotExists(defaultBucket)
		return err
	})
	if err != nil {
		return nil, err
	}
	b := &boltStore{
		path:        path,
		db:          db,
		backupStore: backup,
	}
	b.traverser = traversal.New(b)
	return b, nil
}

func contains(addr swarm.Address, chs ...swarm.Chunk) bool {
	for _, v := range chs {
		if addr.Equal(v.Address()) {
			return true
		}
	}
	return false
}

func (b *boltStore) Info() string {
	if b.backupStore != nil {
		if info, ok := b.backupStore.(store.StoreInfo); ok {
			return fmt.Sprintf("badger store mounted at %s Backup to %s", b.path, info.Info())
		}
	}
	return fmt.Sprintf("badger store mounted at %s", b.path)
}

func (b *boltStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	exist = make([]bool, len(chs))
	txn, err := b.db.Begin(true)
	if err != nil {
		return nil, err
	}

	bkt := txn.Bucket(defaultBucket)

	for i, ch := range chs {
		if contains(ch.Address(), chs[:i]...) {
			exist[i] = true
			continue
		}
		if v := bkt.Get(ch.Address().Bytes()); v != nil {
			exist[i] = true
			continue
		}
		err := bkt.Put(ch.Address().Bytes(), ch.Data())
		if err != nil {
			return nil, err
		}
	}
	if err := txn.Commit(); err != nil {
		return nil, err
	}
	return exist, nil
}

var ErrNotFound = errors.New("not found")

func (b *boltStore) Get(ctx context.Context, mode storage.ModeGet, address swarm.Address) (ch swarm.Chunk, err error) {
	err = b.db.View(func(t *bolt.Tx) error {
		v := t.Bucket(defaultBucket).Get(address.Bytes())
		if v == nil {
			return ErrNotFound
		}
		ch = swarm.NewChunk(address, v)
		return nil
	})
	switch {
	case errors.Is(err, ErrNotFound):
		if b.backupStore != nil {
			return b.backupStore.Get(ctx, mode, address)
		}
	case err != nil:
		return nil, err
	}

	return ch, nil
}

func (b *boltStore) GetItem(i store.Item) error {
	return b.db.View(func(t *bolt.Tx) error {
		v := t.Bucket(defaultBucket).Get(i.Key())
		if v == nil {
			return ErrNotFound
		}
		return i.Unmarshal(v)
	})
}

func (b *boltStore) StoreItem(i store.Item) error {
	buf, err := i.Marshal()
	if err != nil {
		return err
	}
	return b.db.Update(func(t *bolt.Tx) error {
		return t.Bucket(defaultBucket).Put(i.Key(), buf)
	})
}

func (b *boltStore) Backup(ctx context.Context, addr swarm.Address) (string, error) {
	if b.backupStore == nil {
		return "", errors.New("backup store not configured")
	}
	group, cCtx := errgroup.WithContext(ctx)

	addrChan := make(chan swarm.Address)
	chChan := make(chan swarm.Chunk)

	tag, err := b.backupStore.CreateTag(ctx)
	if err != nil {
		return "", err
	}

	group.Go(func() error {
		for {
			select {
			case ch, ok := <-chChan:
				if !ok {
					return nil
				}
				err := b.backupStore.PutWithTag(ctx, storage.ModePutUpload, tag, ch)
				if err != nil {
					return err
				}
			case <-cCtx.Done():
				return cCtx.Err()
			}
		}
	})

	group.Go(func() error {
		defer close(chChan)
		for {
			select {
			case addr, ok := <-addrChan:
				if !ok {
					return nil
				}
				ch, err := b.Get(ctx, storage.ModeGetRequest, addr)
				if err != nil {
					return err
				}
				chChan <- ch
			case <-cCtx.Done():
				return cCtx.Err()
			}
		}
	})
	processAddress := func(ref swarm.Address) error {
		addrChan <- ref
		return nil
	}

	b.traverser.Traverse(ctx, addr, processAddress)
	close(addrChan)

	if err := group.Wait(); err != nil {
		return "", err
	}

	return tag, nil
}

func (b *boltStore) Status(ctx context.Context, tag string) (res store.BackupStat) {
	tags, err := b.backupStore.GetTag(ctx, tag)
	if err != nil {
		return res
	}
	return store.BackupStat(*tags)
}

func (b *boltStore) Close() error {
	return b.db.Close()
}
