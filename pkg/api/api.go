package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/aloknerurkar/bee-fs/pkg/mounter"
	"github.com/aloknerurkar/bee-fs/pkg/store"
	"github.com/aloknerurkar/bee-fs/pkg/store/badger"
	"github.com/aloknerurkar/bee-fs/pkg/store/bbolt"
	"github.com/aloknerurkar/bee-fs/pkg/store/beestore"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	defaultHost     = "localhost"
	defaultHostPort = 1633
)

type httpRouter struct {
	mntr mounter.BeeFsMounter
}

func NewRouter(mntr mounter.BeeFsMounter) *mux.Router {
	r := mux.NewRouter()

	h := &httpRouter{mntr}

	r.HandleFunc("/mount", h.createMount).Methods("POST")
	r.HandleFunc("/mount", h.removeMount).Methods("DELETE")
	r.HandleFunc("/mount", h.getMount).Methods("GET")
	r.HandleFunc("/mounts", h.listMounts).Methods("GET")
	r.HandleFunc("/snapshot", h.createSnapshot).Methods("POST")
	r.HandleFunc("/snapshot", h.getSnapshotInfo).Methods("GET")

	return r
}

type CreateMountRequest struct {
	Path           string
	APIHost        string
	APIHostPort    int
	APIUseSSL      bool
	SnapshotPolicy string
	KeepCount      int
	Encrypt        bool
	Batch          string
	Reference      string
	ReadOnly       bool
	UseBadger      bool
}

func (h *httpRouter) createMount(w http.ResponseWriter, r *http.Request) {
	createReq := &CreateMountRequest{}
	err := decodeJSONBody(w, r, createReq)
	if err != nil {
		var mr *malformedRequest
		if errors.As(err, &mr) {
			http.Error(w, mr.msg, mr.status)
		} else {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	if createReq.APIHost == "" {
		createReq.APIHost = defaultHost
		createReq.APIHostPort = defaultHostPort
	}

	backupStorage := beestore.NewAPIStore(
		createReq.APIHost,
		createReq.APIHostPort,
		createReq.APIUseSSL,
		createReq.Batch,
	)

	var storage store.PutGetter
	if createReq.UseBadger {
		storage, err = badgerstore.NewBadgerStore(createReq.Path, backupStorage.(store.BackupStore))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		storage, err = boltstore.NewBoltStore(createReq.Path, backupStorage.(store.BackupStore))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	opts := []mounter.MountOption{mounter.WithStorage(storage)}

	if createReq.Encrypt {
		opts = append(opts, mounter.WithEncryption())
	}
	if createReq.SnapshotPolicy != "" {
		opts = append(opts, mounter.WithSnapshotSpec(createReq.SnapshotPolicy))
		if createReq.KeepCount == 0 {
			createReq.KeepCount = 5
		}
		opts = append(opts, mounter.WithKeepCount(createReq.KeepCount))
	}

	info, err := h.mntr.Mount(r.Context(), createReq.Path, opts...)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	jsonhttp.Created(w, info)
}

func (h *httpRouter) removeMount(w http.ResponseWriter, r *http.Request) {
	mntPath := r.URL.Query().Get("path")
	if mntPath == "" {
		jsonhttp.BadRequest(w, "mount path not specified")
		return
	}
	err := h.mntr.Unmount(r.Context(), mntPath)
	if err != nil {
		jsonhttp.InternalServerError(w, fmt.Errorf("failed to unmount, reason: %w", err))
		return
	}

	jsonhttp.OK(w, "unmounted "+mntPath)
}

func (h *httpRouter) getMount(w http.ResponseWriter, r *http.Request) {
	mntPath := r.URL.Query().Get("path")
	if mntPath == "" {
		jsonhttp.BadRequest(w, "mount path not specified")
		return
	}
	info, err := h.mntr.Get(mntPath)
	if err != nil {
		jsonhttp.BadRequest(w, err)
		return
	}

	jsonhttp.OK(w, info)
}

func (h *httpRouter) listMounts(w http.ResponseWriter, r *http.Request) {
	mnts := h.mntr.List()

	jsonhttp.OK(w, mnts)
}

type SnapshotResponse struct {
	Reference swarm.Address
}

func (h *httpRouter) createSnapshot(w http.ResponseWriter, r *http.Request) {
	mntPath := r.URL.Query().Get("path")
	if mntPath == "" {
		jsonhttp.BadRequest(w, "mount path not specified")
		return
	}
	ref, err := h.mntr.Snapshot(r.Context(), mntPath)
	if err != nil {
		jsonhttp.BadRequest(w, err)
		return
	}

	jsonhttp.OK(w, &SnapshotResponse{
		Reference: ref,
	})
}

func (h *httpRouter) getSnapshotInfo(w http.ResponseWriter, r *http.Request) {
	mntPath := r.URL.Query().Get("path")
	if mntPath == "" {
		jsonhttp.BadRequest(w, "mount path not specified")
		return
	}
	infos := h.mntr.SnapshotInfo(r.Context(), mntPath)

	jsonhttp.OK(w, infos)
}
