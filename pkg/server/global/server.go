package global

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"m.cluseau.fr/kube-proxy2/pkg/api/localnetv1"
	"m.cluseau.fr/kube-proxy2/pkg/diffstore"
	"m.cluseau.fr/kube-proxy2/pkg/proxystore"
)

type Server struct {
	Store *proxystore.Store
}

var syncItem = &localnetv1.OpItem{Op: &localnetv1.OpItem_Sync{}}

func (s *Server) Watch(res localnetv1.Global_WatchServer) error {
	var rev uint64

	diffs := make([]*diffstore.DiffStore, len(proxystore.AllSets))
	for idx, _ := range proxystore.AllSets {
		diffs[idx] = diffstore.New()
	}

	for {
		if _, err := res.Recv(); err != nil {
			return err
		}

		rev = s.Store.View(rev, func(tx *proxystore.Tx) {
			if !tx.AllSynced() {
				return
			}

			for idx, set := range proxystore.AllSets {
				diff := diffs[idx]
				tx.Each(set, func(kv *proxystore.KV) bool {
					h := kv.Value.GetHash()
					diff.Set([]byte(kv.Key), h, kv.Value)
					return true
				})
			}
		})

		res.Send(syncItem)

		for idx, _ := range proxystore.AllSets {
			diffs[idx].Reset(diffstore.ItemDeleted)
		}
	}
}

type OpConsumer interface {
	Send(op *localnetv1.OpItem) error
}

type WatchState struct {
	res   OpConsumer
	sets  []localnetv1.Set
	diffs []*diffstore.DiffStore
	pb    *proto.Buffer
	Err   error
}

func NewWatchState(res OpConsumer, sets []localnetv1.Set) *WatchState {
	diffs := make([]*diffstore.DiffStore, len(sets))
	for i := range sets {
		diffs[i] = diffstore.New()
	}

	return &WatchState{
		res:   res,
		sets:  sets,
		diffs: diffs,
		pb:    proto.NewBuffer(make([]byte, 0)),
	}
}

func (w *WatchState) StoreFor(set localnetv1.Set) *diffstore.DiffStore {
	for i, s := range w.sets {
		if s == set {
			return w.diffs[i]
		}
	}
	panic(fmt.Errorf("not watching set %v", set))
}

func (w *WatchState) SendUpdates(set localnetv1.Set) (count int) {
	if w.Err != nil {
		return
	}

	store := w.StoreFor(set)

	updated := store.Updated()

	for _, kv := range updated {
		w.sendSet(set, string(kv.Key), kv.Value.(proto.Message))
	}

	return len(updated)
}

func (w *WatchState) SendDeletes(set localnetv1.Set) (count int) {
	if w.Err != nil {
		return
	}

	store := w.StoreFor(set)

	deleted := store.Deleted()

	for _, kv := range deleted {
		w.sendDelete(set, string(kv.Key))
	}

	return len(deleted)
}

func (w *WatchState) send(item *localnetv1.OpItem) {
	if w.Err != nil {
		return
	}
	err := w.res.Send(item)
	if err != nil {
		w.Err = grpc.Errorf(codes.Aborted, "send error: %v", err)
	}
}

func (w *WatchState) sendSet(set localnetv1.Set, path string, m proto.Message) {
	w.pb.Reset()
	if err := w.pb.Marshal(m); err != nil {
		panic("protobuf Marshal failed: " + err.Error())
	}

	w.send(&localnetv1.OpItem{
		Op: &localnetv1.OpItem_Set{
			Set: &localnetv1.Value{
				Ref:   &localnetv1.Ref{Set: set, Path: path},
				Bytes: w.pb.Bytes(),
			},
		},
	})
}

func (w *WatchState) sendDelete(set localnetv1.Set, path string) {
	w.send(&localnetv1.OpItem{
		Op: &localnetv1.OpItem_Delete{
			Delete: &localnetv1.Ref{Set: set, Path: path},
		},
	})
}