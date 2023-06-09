package main

import (
	"fmt"
	"log"
	"strings"
)

type Node struct {
	Key    int32
	Value  int32
	Left   *Node
	Right  *Node
	Parent *Node
}

func NewNode(key int32, value int32) *Node {
	x := &Node{
		Key:    key,
		Value:  value,
		Left:   nil,
		Right:  nil,
		Parent: nil,
	}
	return x
}

func (n *Node) InsertKeyValue(key int32, value int32) {
	x := NewNode(key, value)
	n.Insert(x)
}

func (n *Node) MeanRange(lo int32, hi int32) int32 {
	if hi < lo {
		// "If there are no samples within the requested period,
		// or if mintime comes after maxtime, the value returned must be 0."
		log.Printf("Hi %d < %d Lo", hi, lo)
		return 0
	}

	nums := n.SearchRange(lo, hi)
	length := len(nums)

	if length == 0 {
		// "If there are no samples within the requested period,
		// or if mintime comes after maxtime, the value returned must be 0."
		log.Println("Search yielded no results")
		return 0
	}

	// If we just stick with int32 here, we'll have a problem:
	// the int32s can overflow for a large sum.
	// We could divide as we go, but then we introduce multiple roundings,
	// leading to a growing division error.
	// Instead, we convert to int64 for the computation, then return int32.
	// We shouldn't have to worry about our final int64 overflowing the int32,
	// since the mean of only int32s should also be an int32.
	// (For reference, int32's range is  [-2147483648, 2147483647])
	sum := int64(0)
	for _, v := range nums {
		sum = sum + int64(v)
	}
	return int32(sum / int64(length))
}

func (n *Node) SearchRange(lo int32, hi int32) []int32 {
	out := []int32{}
	if hi < lo {
		// "If there are no samples within the requested period,
		// or if mintime comes after maxtime, the value returned must be 0."
		return out
	}
	q := make([]*Node, 0)
	q = append(q, n)
	for len(q) != 0 {
		this := q[0]
		// Pop
		q = q[1:]
		if this.Key < hi && this.Right != nil {
			q = append(q, this.Right)
		}
		if this.Key > lo && this.Left != nil {
			q = append(q, this.Left)
		}
		if lo <= this.Key && this.Key <= hi {
			out = append(out, this.Value)
		}
	}

	return out
}

func (n *Node) Insert(z *Node) {
	x := n
	for x != nil {
		if z.Key < x.Key {
			// Go left
			if x.Left == nil {
				// Insert
				x.Left = z
				z.Parent = x
				return
			} else {
				// Continue left
				x = x.Left
			}
		} else if z.Key > x.Key {
			// Go right
			if x.Right == nil {
				// Insert
				x.Right = z
				z.Parent = x
				return
			} else {
				// Continue right
				x = x.Right
			}
		} else {
			// Equal :( Undefined behavior for spec.
			// Easiest thing is to do nothing.
			return
		}
	}
}

func (n *Node) Text() {
	//TODO Currently printing, not returning strings
	return
}

func (n *Node) Show() {
	n.indented(0)
}

func (n *Node) indented(depth int) {
	indent := strings.Repeat("\t", depth)
	fmt.Printf("%s%d:%d\n", indent, n.Key, n.Value)
	// Show right then left, since
	// root is on left of screen
	// "top" is right of tree
	// "bottom" is left of tree
	if n.Right == nil {
		fmt.Printf("%sRight:nil\n", indent+"\t")
	} else {
		n.Right.indented(depth + 1)
	}
	if n.Left == nil {
		fmt.Printf("%sLeft:nil\n", indent+"\t")
	} else {
		n.Left.indented(depth + 1)
	}
}
