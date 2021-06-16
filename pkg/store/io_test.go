// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	cmdfile "github.com/aloknerurkar/bee-fs/pkg/file"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/logging"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/tags"
)

// TestAPIStore verifies that the api store layer does not distort data, and that same
// data successfully posted can be retrieved from http backend.
func TestAPIStore(t *testing.T) {
	storer := mock.NewStorer()
	ctx := context.Background()
	srvUrl := newTestServer(t, storer)

	host := srvUrl.Hostname()
	port, err := strconv.Atoi(srvUrl.Port())
	if err != nil {
		t.Fatal(err)
	}
	a := cmdfile.NewAPIStore(host, port, false)

	ch := testingc.GenerateTestRandomChunk()
	_, err = a.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = storer.Get(ctx, storage.ModeGetRequest, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	chResult, err := a.Get(ctx, storage.ModeGetRequest, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Equal(chResult) {
		t.Fatal("chunk mismatch")
	}
}

// TestLimitWriter verifies that writing will fail when capacity is exceeded.
func TestLimitWriter(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	data := []byte("foo")
	writeCloser := cmdfile.NopWriteCloser(buf)
	w := cmdfile.NewLimitWriteCloser(writeCloser, int64(len(data)))
	c, err := w.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if c < 3 {
		t.Fatal("short write")
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("expected written data %x, got %x", data, buf.Bytes())
	}
	_, err = w.Write(data[:1])
	if err == nil {
		t.Fatal("expected overflow error")
	}
}

// newTestServer creates an http server to serve the bee http api endpoints.
func newTestServer(t *testing.T, storer storage.Storer) *url.URL {
	t.Helper()
	logger := logging.New(ioutil.Discard, 0)
	store := statestore.NewStateStore()
	s := api.New(tags.NewTags(store, logger), storer, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger, nil, api.Options{})
	ts := httptest.NewServer(s)
	srvUrl, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	return srvUrl
}
