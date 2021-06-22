package badgerstore

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/errgroup"

	"github.com/aloknerurkar/bee-fs/pkg/store"
	badger "github.com/dgraph-io/badger/v3"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
)

type badgerStore struct {
	path        string
	db          *badger.DB
	traverser   traversal.Traverser
	backupStore store.BackupStore
}

func NewBadgerStore(path string, backup store.BackupStore) (store.PutGetter, error) {
	db, err := badger.Open(badger.DefaultOptions(path))
	if err != nil {
		return nil, err
	}
	b := &badgerStore{
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

func (b *badgerStore) Info() string {
	if b.backupStore != nil {
		if info, ok := b.backupStore.(store.StoreInfo); ok {
			return fmt.Sprintf("badger store mounted at %s Backup to %s", b.path, info.Info())
		}
	}
	return fmt.Sprintf("badger store mounted at %s", b.path)
}

func (b *badgerStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	exist = make([]bool, len(chs))
	rdr := b.db.NewTransaction(false)
	wb := b.db.NewWriteBatch()

	for i, ch := range chs {
		if contains(ch.Address(), chs[:i]...) {
			exist[i] = true
			continue
		}
		if _, err := rdr.Get(ch.Address().Bytes()); err == nil {
			exist[i] = true
			continue
		}
		err := wb.Set(ch.Address().Bytes(), ch.Data())
		if err != nil {
			return nil, err
		}
	}
	if err := wb.Flush(); err != nil {
		return nil, err
	}
	return exist, nil
}

func (b *badgerStore) Get(ctx context.Context, mode storage.ModeGet, address swarm.Address) (ch swarm.Chunk, err error) {
	rdr := b.db.NewTransaction(false)

	item, err := rdr.Get(address.Bytes())
	if err != nil {
		return nil, err
	}
	var valCopy []byte
	err = item.Value(func(val []byte) error {
		valCopy = append([]byte{}, val...)
		return nil
	})
	return swarm.NewChunk(address, valCopy), nil
}

func (b *badgerStore) GetItem(i store.Item) error {
	return b.db.View(func(t *badger.Txn) error {
		item, err := t.Get(i.Key())
		if err != nil {
			return err
		}
		var valCopy []byte
		err = item.Value(func(val []byte) error {
			valCopy = append([]byte{}, val...)
			return nil
		})
		return i.Unmarshal(valCopy)
	})
}

func (b *badgerStore) StoreItem(i store.Item) error {
	buf, err := i.Marshal()
	if err != nil {
		return err
	}
	return b.db.Update(func(t *badger.Txn) error {
		return t.Set(i.Key(), buf)
	})
}

func (b *badgerStore) Backup(ctx context.Context, addr swarm.Address) (string, error) {
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
		return nil
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
		return nil
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

func (b *badgerStore) Status(ctx context.Context, tag string) (res store.BackupStat) {
	tags, err := b.backupStore.GetTag(ctx, tag)
	if err != nil {
		return res
	}
	return store.BackupStat(*tags)
}

func (b *badgerStore) Close() error {
	return b.db.Close()
}
