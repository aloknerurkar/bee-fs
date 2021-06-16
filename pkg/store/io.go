// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// nopWriteCloser wraps a io.Writer in the same manner as ioutil.NopCloser does
// with an io.Reader.
type nopWriteCloser struct {
	io.Writer
}

// NopWriteCloser returns a new io.WriteCloser with the given writer as io.Writer.
func NopWriteCloser(w io.Writer) io.WriteCloser {
	return &nopWriteCloser{
		Writer: w,
	}
}

// Close implements io.Closer
func (n *nopWriteCloser) Close() error {
	return nil
}

// PutGetter wraps both storage.Putter and storage.Getter interfaces
type PutGetter interface {
	storage.Putter
	storage.Getter
	Info() string
}

// APIStore provies a storage.Putter that adds chunks to swarm through the HTTP chunk API.
type APIStore struct {
	Client  *http.Client
	baseUrl string
}

// NewAPIStore creates a new APIStore.
func NewAPIStore(host string, port int, tls bool) PutGetter {
	scheme := "http"
	if tls {
		scheme += "s"
	}
	u := &url.URL{
		Host:   fmt.Sprintf("%s:%d", host, port),
		Scheme: scheme,
		Path:   "chunks",
	}
	return &APIStore{
		Client:  http.DefaultClient,
		baseUrl: u.String(),
	}
}

// Put implements storage.Putter.
func (a *APIStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for _, ch := range chs {
		buf := bytes.NewReader(ch.Data())
		url := strings.Join([]string{a.baseUrl}, "/")
		req, err := http.NewRequestWithContext(ctx, "POST", url, buf)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		res, err := a.Client.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("upload failed: %v", res.Status)
		}
	}
	exist = make([]bool, len(chs))
	return exist, nil
}

// Get implements storage.Getter.
func (a *APIStore) Get(ctx context.Context, mode storage.ModeGet, address swarm.Address) (ch swarm.Chunk, err error) {
	addressHex := address.String()
	url := strings.Join([]string{a.baseUrl, addressHex}, "/")
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	res, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chunk %s not found", addressHex)
	}
	chunkData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	ch = swarm.NewChunk(address, chunkData)
	return ch, nil
}

func (a *APIStore) Info() string {
	return a.baseUrl
}

// LimitWriteCloser limits the output from the application.
type LimitWriteCloser struct {
	io.WriteCloser
	total int64
	limit int64
}

// NewLimitWriteCloser creates a new LimitWriteCloser.
func NewLimitWriteCloser(w io.WriteCloser, c int64) io.WriteCloser {
	return &LimitWriteCloser{
		WriteCloser: w,
		limit:       c,
	}
}

// Write implements io.Writer.
func (l *LimitWriteCloser) Write(b []byte) (int, error) {
	if l.total+int64(len(b)) > l.limit {
		return 0, errors.New("overflow")
	}
	c, err := l.WriteCloser.Write(b)
	l.total += int64(c)
	return c, err
}

func SetLogger(cmd *cobra.Command, verbosityString string) (logger logging.Logger, err error) {
	v := strings.ToLower(verbosityString)
	switch v {
	case "0", "silent":
		logger = logging.New(ioutil.Discard, 0)
	case "1", "error":
		logger = logging.New(cmd.OutOrStderr(), logrus.ErrorLevel)
	case "2", "warn":
		logger = logging.New(cmd.OutOrStderr(), logrus.WarnLevel)
	case "3", "info":
		logger = logging.New(cmd.OutOrStderr(), logrus.InfoLevel)
	case "4", "debug":
		logger = logging.New(cmd.OutOrStderr(), logrus.DebugLevel)
	case "5", "trace":
		logger = logging.New(cmd.OutOrStderr(), logrus.TraceLevel)
	default:
		return nil, fmt.Errorf("unknown verbosity level %q", v)
	}
	return logger, nil
}
