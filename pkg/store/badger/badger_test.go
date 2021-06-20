package badgerstore_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/aloknerurkar/bee-fs/pkg/store/badger"
	storetesting "github.com/aloknerurkar/bee-fs/pkg/store/testing"
)

func TestBadgerStore(t *testing.T) {
	mntDir, err := ioutil.TempDir("", "tmpbadger")
	st, err := badgerstore.NewBadgerStore(mntDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mntDir)

	storetesting.RunTests(t, st)
}
