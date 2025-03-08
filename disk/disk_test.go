// Package disk tests
// Copyright (C) 2025 Alex Gaetano Padula & VFSLite Contributors
//
// This library is free software; you can redistribute it and/or
// modify it under the terms of the GNU Lesser General Public
// License as published by the Free Software Foundation; either
// version 2.1 of the License, or (at your option) any later version.
//
// This library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
// Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public
// License along with this library; if not, write to the Free Software
// Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301  USA
package disk

import (
	"os"
	"testing"
)

func TestOpenNewDisk(t *testing.T) {
	testFileName := "test.vfsl"

	defer os.Remove(testFileName)

	disk, err := Open(testFileName, os.O_RDWR|os.O_CREATE, 0777, 4096, 100, 2, 0.75)
	if err != nil {
		t.Fatalf("Failed to open new disk: %v", err)
	}
	defer disk.Close()

	if disk.header.signature != Signature {
		t.Errorf("Expected signature %x, got %x", Signature, disk.header.signature)
	}
	if disk.header.version != 1 {
		t.Errorf("Expected version 1, got %d", disk.header.version)
	}
	if disk.header.blockSize != 4096 {
		t.Errorf("Expected block size 4096, got %d", disk.header.blockSize)
	}
	if disk.header.totalBlocks != 100 {
		t.Errorf("Expected total blocks 100, got %d", disk.header.totalBlocks)
	}
}
