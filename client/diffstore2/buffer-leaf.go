package diffstore2

import (
	"bytes"

	"github.com/cespare/xxhash"
)

func NewBufferStore() *Store[string, *BufferLeaf] {
	return New[string](NewBufferLeaf)
}

type BufferLeaf struct {
	bytes.Buffer
}

func NewBufferLeaf() *BufferLeaf {
	return &BufferLeaf{bytes.Buffer{}}
}

var _ Leaf = NewBufferLeaf()

func (l *BufferLeaf) Reset() {
	l.Buffer.Reset()
}

func (l *BufferLeaf) Hash() uint64 {
	return xxhash.Sum64(l.Bytes())
}
