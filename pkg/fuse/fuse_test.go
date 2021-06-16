package fs_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/aloknerurkar/bee-fs/pkg/fuse"
	"github.com/ethersphere/bee/pkg/storage/mock"
	logger "github.com/ipfs/go-log/v2"
)

func TestFuseFs(t *testing.T) {
	logger.SetLogLevel("*", "Debug")
	mntDir, err := ioutil.TempDir("", "tmpfuse")
	if err != nil {
		t.Fatalf("failed creating dir %s", err.Error())
	}
	defer os.RemoveAll(mntDir)
	fsImpl, err := fs.New(mock.NewStorer())
	if err != nil {
		t.Fatalf("failed creating new fs %s", err.Error())
	}
	srv := fuse.NewFileSystemHost(fsImpl)
	srv.SetCapReaddirPlus(true)
	go func() {
		if !srv.Mount(mntDir, []string{"-d"}) {
			t.Error("failed mounting")
		}
	}()
	defer srv.Unmount()

	time.Sleep(time.Second)

	FileBasic(t, mntDir)
}

func FileBasic(t *testing.T, mnt string) {
	content := []byte("hello world")
	fn := mnt + "/file"

	if err := ioutil.WriteFile(fn, content, 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got, err := ioutil.ReadFile(fn); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if bytes.Compare(got, content) != 0 {
		t.Fatalf("ReadFile: got %q, want %q", got, content)
	}

	f, err := os.Open(fn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Fstat: %v", err)
	} else if int(fi.Size()) != len(content) {
		t.Errorf("got size %d want 5", fi.Size())
	}
	if got, want := uint32(fi.Mode()), uint32(0755); got != want {
		t.Errorf("Fstat: got mode %o, want %o", got, want)
	}
	if err := f.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

}
