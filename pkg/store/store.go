// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store

import (
	"context"
	"time"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// PutGetter wraps both storage.Putter and storage.Getter interfaces
type PutGetter interface {
	storage.Putter
	storage.Getter
}

type StoreInfo interface {
	Info() string
}

type Serializable interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
}

type Item interface {
	Serializable

	Key() []byte
}

type ItemStore interface {
	GetItem(Item) error
	StoreItem(Item) error
}

type BackupStat TagInfo

type Backuper interface {
	Backup(context.Context, swarm.Address) (string, error)
	Status(context.Context, string) BackupStat
}

type TagInfo struct {
	Uid       uint32
	StartedAt time.Time
	Total     int64
	Processed int64
	Synced    int64
}

type BackupStore interface {
	storage.Getter

	PutWithTag(context.Context, string, <-chan swarm.Chunk) error
	CreateTag(context.Context) (string, error)
	GetTag(context.Context, string) (*TagInfo, error)
}
