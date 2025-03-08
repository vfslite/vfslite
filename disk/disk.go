// Package disk
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

const Signature uint32 = 0x5646534C // VFSL

// Header is a struct for the disk header
type Header struct {
	Signature        uint32 // Magic number to identify our disk format (VFSL)
	Version          uint16 // Version of the disk format
	BlockSize        uint32 // Size of each block in bytes.  This is determined by the user
	TotalBlocks      uint64 // Total number of blocks currently in the disk
	AllocSetOffset   uint64 // Offset to the allocation bitmap
	AllocSetSize     uint64 // Size of allocation bitmap in bytes
	DataBlocksOffset uint64 // Offset to the first data block
}
