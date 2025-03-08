// Package vfslite
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
package vfslite

import "vfslite/disk"

// Block types
const (
	BlockTypeRoot      = 1
	BlockTypeDirectory = 2
	BlockTypeFile      = 3
	BlockTypeData      = 4
	HeaderSize         = 9    // size of Type(1) + ChildCount(2) + DataSize(4) + MetaSize(2)
	ReferenceSize      = 8    // Size of each block reference (uint64)
	DebugEnabled       = true // Set to true to enable debug output
	SuperblockNum      = 0    // Block 0 is reserved for the superblock
)

// BlockHeader defines block specific metadata at the beginning of each block
type BlockHeader struct {
	Type       uint8  // Type of block (e.g., directory, file, etc.)
	ChildCount uint16 // Number of child references
	DataSize   uint32 // Size of data in this block (excluding header and references)
	MetaSize   uint16 // Size of metadata in bytes
}

// Metadata defines common metadata for all file and directory blocks
type Metadata struct {
	CreatedOn  int64             `json:"createdOn"`  // Unix timestamp for creation time
	ModifiedOn int64             `json:"modifiedOn"` // Unix timestamp for last modification
	Extra      map[string]string `json:"extra"`      // Optional extra metadata as key-value pairs
} // JSON works fine :)

// VFSLite main structure for the virtual filesystem
type VFSLite struct {
	disk      *disk.Disk // Underlying disk structure
	blockSize uint       // Size of each block in bytes (configured by user)
	rootBlock uint64     // The entry point to our disk structure
}
