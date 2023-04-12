package main

import (
	"fmt"
	"sync"
)

/**
* It's just the Sieve of Eratosthenes. Nothing clever.
* Mutex not actually needed for current usage;
* just didn't want to leave a footgun lying around.
*
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
	// Don't allow IsPrime() to be called during Solve()
	mu sync.Mutex
	// primeList is a list of size (max+1) to allow a 1:1 relationship between
	// indices and integers. i.e. primeList[7] is true, primeList[8] is false.
	primeList []bool
	max       int
}

// IsPrime checks if n is pre-computed in s.primeList, but errors if n > s.max.
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

// Solve will pre-compute primes up to s.max using the Sieve of Eratosthenes.
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

// NewSieve creates a Sieve and pre-computes the primes up to and including solveTo.
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
