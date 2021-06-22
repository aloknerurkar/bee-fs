package beestore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/aloknerurkar/bee-fs/pkg/store"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// APIStore provies a storage.Putter that adds chunks to swarm through the HTTP chunk API.
type APIStore struct {
	Client  *http.Client
	baseUrl string
	tagUrl  string
	batch   string
}

// NewAPIStore creates a new APIStore.
func NewAPIStore(host string, port int, tls bool, batch string) store.PutGetter {
	scheme := "http"
	if tls {
		scheme += "s"
	}
	u := &url.URL{
		Host:   fmt.Sprintf("%s:%d", host, port),
		Scheme: scheme,
		Path:   "chunks",
	}
	t := &url.URL{
		Host:   fmt.Sprintf("%s:%d", host, port),
		Scheme: scheme,
		Path:   "tags",
	}
	return &APIStore{
		Client:  http.DefaultClient,
		baseUrl: u.String(),
		tagUrl:  t.String(),
		batch:   batch,
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
		req.Header.Set("swarm-postage-batch-id", a.batch)
		res, err := a.Client.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != http.StatusCreated {
			return nil, fmt.Errorf("upload failed: %v", res.Status)
		}
		res.Body.Close()
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
	defer res.Body.Close()
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

func (a *APIStore) PutWithTag(ctx context.Context, mode storage.ModePut, tag string, chs ...swarm.Chunk) (err error) {
	for _, ch := range chs {
		buf := bytes.NewReader(ch.Data())
		url := strings.Join([]string{a.baseUrl}, "/")
		req, err := http.NewRequestWithContext(ctx, "POST", url, buf)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Swarm-Postage-Batch-Id", a.batch)
		req.Header.Set("Swarm-Tag", tag)
		res, err := a.Client.Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusCreated {
			return fmt.Errorf("upload failed: %v", res.Status)
		}
		res.Body.Close()
	}
	return nil
}

func (a *APIStore) CreateTag(ctx context.Context) (string, error) {
	url := strings.Join([]string{a.tagUrl}, "/")
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", err
	}
	res, err := a.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	tags := &store.TagInfo{}
	err = json.Unmarshal(buf, tags)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(int(tags.Uid)), nil
}

func (a *APIStore) GetTag(ctx context.Context, tag string) (*store.TagInfo, error) {
	url := strings.Join([]string{a.tagUrl, tag}, "/")
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	res, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	tags := &store.TagInfo{}
	err = json.Unmarshal(buf, tags)
	if err != nil {
		return nil, err
	}
	return tags, nil
}
