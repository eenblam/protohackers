package main

import (
	"testing"
)

func TestMix(t *testing.T) {
	inserts := [][]int32{
		[]int32{319380318, 4520},
		[]int32{319416158, 4527},
		[]int32{319467670, 4521},
		[]int32{319561000, 4542},
		[]int32{319641912, 4551},
		[]int32{319686555, 4567},
		[]int32{319771920, 4590},
		[]int32{319838481, 4587},
		[]int32{19140449, 6098},
		[]int32{19240359, 6097},
		[]int32{247208201, 8190},
		[]int32{247302502, 8173},
		[]int32{927166283, -475},
		[]int32{927217931, -461},
		[]int32{927292768, -464},
		[]int32{927328411, -459},
	}
	var bt *Node
	for _, i := range inserts {
		if bt == nil {
			bt = NewNode(i[0], i[1])
		} else {
			bt.InsertKeyValue(i[0], i[1])
		}
	}
	got := bt.MeanRange(927284767, 927321905)
	expected := int32(-464)
	if got != expected {
		bt.Show()
		t.Fatalf("Expected %d got %d", expected, got)
	}
}

func TestPositive(t *testing.T) {
	inserts := [][]int32{
		[]int32{388967869, 6993},
		[]int32{389067081, 6979},
		[]int32{389118352, 6969},
		[]int32{389133639, 6979},
		[]int32{389196453, 6965},
		[]int32{389266708, 6960},
		[]int32{389285955, 6972},
		[]int32{389372810, 6966},
		[]int32{389427516, 6951},
		[]int32{389484837, 6957},
		[]int32{389580546, 6965},
		[]int32{389652137, 6950},
		[]int32{389682972, 6954},
		[]int32{389750179, 6952},
	}
	var bt *Node
	for _, i := range inserts {
		if bt == nil {
			bt = NewNode(i[0], i[1])
		} else {
			bt.InsertKeyValue(i[0], i[1])
		}
	}
	got := bt.MeanRange(389284017, 389447149)
	// Expect 6963 = 20889/3
	expected := int32(6963)
	if got != expected {
		bt.Show()
		t.Fatalf("Expected %d got %d", expected, got)
	}
}
