package types

type RingBuffer[T any] struct {
	buf []T

	head int
	tail int
}

func NewRingBuffer[T any](capacity ...int) *RingBuffer[T] {
	c := 16
	if len(capacity) > 0 && capacity[0] > 0 {
		c = capacity[0]
	}

	return &RingBuffer[T]{
		buf:  make([]T, c),
		head: 0,
		tail: 0,
	}
}

func (b *RingBuffer[T]) Capacity() int {
	return cap(b.buf)
}

func (b *RingBuffer[T]) Length() int {
	switch {
	case b.head >= b.tail:
		return b.head - b.tail
	case b.head < b.tail:
		return len(b.buf) + b.head - b.tail
	default:
		return 0
	}
}

func (b *RingBuffer[T]) Push(value T) {
	switch {
	case b.head >= b.tail:
		//  0   1   2   3
		//  t       h      : 2 elements
		size := b.head - b.tail
		if size == len(b.buf)-1 { // time to grow
			newBuf := make([]T, 2*len(b.buf))
			copy(newBuf, b.buf)
			b.buf = newBuf
		}
		b.buf[b.head] = value
		b.head++
		if b.head == len(b.buf) {
			b.head = 0
		}
	case b.head < b.tail:
		//  0   1   2   3
		//  h           t  : 1 element
		size := len(b.buf) + b.head - b.tail
		if size == len(b.buf)-1 { // time to grow
			newBuf := make([]T, 2*len(b.buf))
			copy(newBuf, b.buf[b.tail:])
			copy(newBuf[len(b.buf)-b.tail:], b.buf[:b.head])
			b.buf = newBuf
			b.tail = 0
			b.head = size
		}
		b.buf[b.head] = value
		b.head++
	}
}

func (b *RingBuffer[T]) Pop() (T, bool) {
	if b.head == b.tail {
		var res T // nil value
		return res, false
	}

	v := b.buf[b.tail]
	b.tail++
	if b.tail == len(b.buf) {
		b.tail = 0
	}

	return v, true
}

func (b *RingBuffer[T]) Pick() (T, bool) {
	if b.head == b.tail {
		var res T // nil value
		return res, false
	}

	b.head--
	if b.head < 0 {
		b.head += len(b.buf)
	}

	return b.buf[b.head], true
}

func (b *RingBuffer[T]) Forget(count int) {
	if count >= b.Length() {
		b.head = 0
		b.tail = 0
		return
	}

	b.head -= count
	if b.head < 0 {
		b.head += len(b.buf)
	}
}

func (b *RingBuffer[T]) At(idx int) (T, bool) {
	if idx >= b.Length() {
		var res T // nil value
		return res, false
	}

	pos := b.tail + idx
	if pos >= len(b.buf) {
		pos -= len(b.buf)
	}

	return b.buf[pos], true
}
