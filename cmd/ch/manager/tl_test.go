package manager

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"
)

func TestTLCache(t *testing.T) {
	rand.Seed(time.Now().Unix())
	n := rand.Intn(50) + 50
	c := &tlCache{max: n}

	vs := []int{}
	for i := 0; i < n; i++ {
		x := rand.Intn(1e8)
		vs = append(vs, x)
		v := fmt.Sprintf("%09d", x)
		c.add(v)
	}

	sort.Ints(vs)
	t.Log(vs)

	for i := range c.u {
		v := fmt.Sprintf("%09d", vs[i])
		if v != c.u[i] {
			t.Fatal(i, c.u, vs)
		}
	}

	for i, v := range vs {
		p, _ := strconv.Atoi(c.findPrev(fmt.Sprintf("%09d", v)))
		if i == len(vs)-1 {
			if p != 0 {
				t.Fatal(vs, p, i)
			}
		} else {
			if p != vs[i+1] {
				t.Fatal(vs, p, i)
			}
		}
	}

	gap := vs[len(vs)-1] - vs[0]
	for i := vs[0] - 1; i < vs[len(vs)-1]+1; i += gap / n {
		p, _ := strconv.Atoi(c.findPrev(fmt.Sprintf("%09d", i)))
		if p < i {
			t.Fatal(p, i)
		}
	}
}
