package main

import (
	"fmt"
	"sync"
)

/**
* IDEA:
* Keep a fixed size array for running computations
* Sieve that space, then add everything remaining to a map when done
* For subsequent sieves:
*   * re-seed that array
*   * then sieve from map
*   * then sieve from what's left in array
*   * finally, add array elements to map
*
* hmmmmm okay first a simpler idea:
* Pre-compute it all once and see if it works. :P
 */

type Sieve struct {
	mu        sync.Mutex
	primeList []bool
	max       int
}

func (s *Sieve) IsPrime(n int) (bool, error) {
	if n < 2 {
		return false, nil
	}
	if n > s.max {
		return false, fmt.Errorf("Only computed to %d, got %d", s.max, n)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.primeList[n], nil
}

func (s *Sieve) Solve() {
	s.mu.Lock()
	defer s.mu.Unlock()
	half := (s.max + 1) / 2
	for i := 0; i < half; i++ {
		if !s.primeList[i] {
			continue
		}
		// Increment FIRST, so that we don't mark the prime itself false
		for k := 2 * i; k <= s.max; k += i {
			s.primeList[k] = false
		}
	}
}

func NewSieve(solveTo int) (*Sieve, error) {
	if solveTo < 2 {
		return nil, fmt.Errorf("Expected solveTo >= 2, got %s", solveTo)
	}
	// Allocate with size +1 so that the index refers exactly to the number in question
	// Don't have to constantly add/subtract 1 this way
	l := make([]bool, solveTo+1)
	for i, _ := range l {
		l[i] = true
	}
	l[0] = false
	l[1] = false

	s := &Sieve{
		primeList: l,
		max:       solveTo,
	}

	s.Solve()
	return s, nil
}
