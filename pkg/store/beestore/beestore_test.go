// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package beestore_test

import (
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/aloknerurkar/bee-fs/pkg/store"
	"github.com/aloknerurkar/bee-fs/pkg/store/beestore"
	storetesting "github.com/aloknerurkar/bee-fs/pkg/store/testing"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

// TestAPIStore verifies that the api store layer does not distort data, and that same
// data successfully posted can be retrieved from http backend.
func TestAPIStore(t *testing.T) {
	srvUrl := newTestServer(t, mock.NewStorer())

	host := srvUrl.Hostname()
	port, err := strconv.Atoi(srvUrl.Port())
	if err != nil {
		t.Fatal(err)
	}
	bId := swarm.NewAddress(postagetesting.MustNewID()).String()
	st := beestore.NewAPIStore(host, port, false, bId)

	storetesting.RunTests(t, st)
}

func TestBackupStore(t *testing.T) {
	srvUrl := newTestServer(t, mock.NewStorer())

	host := srvUrl.Hostname()
	port, err := strconv.Atoi(srvUrl.Port())
	if err != nil {
		t.Fatal(err)
	}
	bId := swarm.NewAddress(postagetesting.MustNewID()).String()
	st := beestore.NewAPIStore(host, port, false, bId)

	bkpSt := st.(store.BackupStore)

	storetesting.RunBackupTests(t, bkpSt)
}

// newTestServer creates an http server to serve the bee http api endpoints.
func newTestServer(t *testing.T, storer storage.Storer) *url.URL {
	t.Helper()
	logger := logging.New(ioutil.Discard, 0)
	store := statestore.NewStateStore()
	pk, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(pk)
	s := api.New(
		tags.NewTags(store, logger),
		storer,
		nil, nil, nil, nil, nil,
		mockpost.New(mockpost.WithAcceptAll()),
		nil, nil,
		signer,
		logger,
		nil,
		api.Options{},
	)
	ts := httptest.NewServer(s)
	srvUrl, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	return srvUrl
}
