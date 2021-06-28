package testing

import (
	"context"
	"golang.org/x/sync/errgroup"
	"strconv"
	"testing"

	"github.com/aloknerurkar/bee-fs/pkg/store"
	"github.com/ethersphere/bee/pkg/storage"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

func RunTests(t *testing.T, st store.PutGetter) {
	t.Helper()

	ctx := context.Background()

	ch := testingc.GenerateTestRandomChunk()
	_, err := st.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}
	chResult, err := st.Get(ctx, storage.ModeGetRequest, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Equal(chResult) {
		t.Fatal("chunk mismatch")
	}
}

func RunBackupTests(t *testing.T, st store.BackupStore) {
	t.Helper()

	ctx := context.Background()

	tag, err := st.CreateTag(ctx)
	if err != nil {
		t.Fatal(err)
	}

	tagInfo, err := st.GetTag(ctx, tag)
	if err != nil {
		t.Fatal(err)
	}

	if strconv.Itoa(int(tagInfo.Uid)) != tag {
		t.Fatalf("invalid tag returned on initial get exp %s found %s", tag, strconv.Itoa(int(tagInfo.Uid)))
	}

	if tagInfo.Total != 0 || tagInfo.Synced != 0 || tagInfo.Processed != 0 {
		t.Fatalf("incorrect tag details %+v", tagInfo)
	}

	group := errgroup.Group{}
	chChan := make(chan swarm.Chunk)
	group.Go(func() error {
		return st.PutWithTag(ctx, tag, chChan)
	})
	chsToGet := []swarm.Chunk{}
	for i := 0; i < 2; i++ {
		ch := testingc.GenerateTestRandomChunk()
		chChan <- ch
		chsToGet = append(chsToGet, ch)
	}
	close(chChan)
	err = group.Wait()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range chsToGet {
		chResult, err := st.Get(ctx, storage.ModeGetRequest, v.Address())
		if err != nil {
			t.Fatal(err)
		}
		if !v.Equal(chResult) {
			t.Fatal("chunk mismatch")
		}
	}

	tagInfo, err = st.GetTag(ctx, tag)
	if err != nil {
		t.Fatal(err)
	}

	if strconv.Itoa(int(tagInfo.Uid)) != tag {
		t.Fatalf("invalid tag returned exp %s found %s", tag, strconv.Itoa(int(tagInfo.Uid)))
	}

	if tagInfo.Total != 0 || tagInfo.Synced != 0 || tagInfo.Processed != 2 {
		t.Fatalf("incorrect tag details %+v", tagInfo)
	}
}
