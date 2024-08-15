package priorityqueue

import (
	"math/rand"
	"slices"
	"testing"
)

func TestBasicQueueWithStrings(t *testing.T) {
	stringLenCmp := func(a, b string) int {
		return len(a) - len(b)
	}

	pq := New(stringLenCmp)

	assertPopAndSize := func(s string, n int) {
		t.Helper()
		got := pq.PopMax()
		if got != s {
			t.Errorf("got %v, want %v", got, s)
		}
		if n != pq.Len() {
			t.Errorf("got len=%v, want %v", pq.Len(), n)
		}
	}

	pq.Insert("one")
	pq.Insert("four")
	pq.Insert("sixteen")
	pq.Insert("un")

	// Pop all elements in max order
	assertPopAndSize("sixteen", 3)
	assertPopAndSize("four", 2)
	assertPopAndSize("one", 1)
	assertPopAndSize("un", 0)

	// Insert+pop, insert+pop...
	pq.Insert("xyz")
	assertPopAndSize("xyz", 0)
	pq.Insert("foobarbaz")
	assertPopAndSize("foobarbaz", 0)
	pq.Insert("1")
	assertPopAndSize("1", 0)

	// Inserts after popping some
	pq.Insert("mercury")
	pq.Insert("venus")
	assertPopAndSize("mercury", 1)
	pq.Insert("jupiter")
	assertPopAndSize("jupiter", 1)
	pq.Insert("moon")
	assertPopAndSize("venus", 1)
	assertPopAndSize("moon", 0)

	// Insert two, pop 1, a few times
	pq.Insert("mercury")
	pq.Insert("venus")
	assertPopAndSize("mercury", 1)
	pq.Insert("mars")
	pq.Insert("jupiter")
	assertPopAndSize("jupiter", 2) // contains: venus, mars
	pq.Insert("ganimede")
	pq.Insert("europa")
	assertPopAndSize("ganimede", 3) // contains: venus, mars, europa
	pq.Insert("enceladus")
	pq.Insert("io")
	assertPopAndSize("enceladus", 4)
	assertPopAndSize("europa", 3)
	assertPopAndSize("venus", 2)
	assertPopAndSize("mars", 1)
	assertPopAndSize("io", 0)

	// Insert these words in random orders; they should still all pop in the
	// expected order by length.
	words := []string{"z", "xy", "uvw", "post", "dworb"}
	for i := 0; i < 100; i++ {
		w := slices.Clone(words)
		rand.Shuffle(len(w), func(i, j int) {
			w[i], w[j] = w[j], w[i]
		})

		for _, word := range w {
			pq.Insert(word)
		}

		assertPopAndSize("dworb", 4)
		assertPopAndSize("post", 3)
		assertPopAndSize("uvw", 2)
		assertPopAndSize("xy", 1)
		assertPopAndSize("z", 0)
	}
}

func TestBasicQueueWithCustomType(t *testing.T) {
	type Item struct {
		Name string
		Cost int
	}

	itemCostCmp := func(a, b Item) int {
		return a.Cost - b.Cost
	}

	pq := New(itemCostCmp)

	assertPop := func(s string) {
		t.Helper()
		got := pq.PopMax()
		if got.Name != s {
			t.Errorf("got %v, want %v", got.Name, s)
		}
	}

	// Push in decreasing cost order
	pq.Insert(Item{"joe", 20})
	pq.Insert(Item{"maxm", 3})
	pq.Insert(Item{"jabbar", 1})
	assertPop("joe")
	assertPop("maxm")
	assertPop("jabbar")

	// Push in increasing cost order
	pq.Insert(Item{"x", 1})
	pq.Insert(Item{"y", 29})
	pq.Insert(Item{"z", 88})
	assertPop("z")
	assertPop("y")
	assertPop("x")
}
