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

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
)

const Signature uint32 = 0x5646534C // VFSL

// HeaderSize is the size of the fixed header structure
const HeaderSize = 4 + 2 + 4 + 8 + 8 + 8 + 8 // 42 bytes

// Header is a struct for the disk header
type Header struct {
	signature        uint32 // Magic number to identify our disk format (VFSL)
	version          uint16 // Version of the disk format
	blockSize        uint32 // Size of each block in bytes.  This is determined by the user
	totalBlocks      uint64 // Total number of blocks currently in the disk
	allocSetOffset   uint64 // Offset to the allocation bitmap
	allocSetSize     uint64 // Size of allocation bitmap in bytes
	dataBlocksOffset uint64 // Offset to the first data block
}

// Disk is a struct for the low level disk (part of virtual disk imp)
type Disk struct {
	file              *os.File        // Open descriptor for the disk file
	header            Header          // Header for the disk
	blockLocks        []*sync.RWMutex // Locks for each block
	lock              *sync.RWMutex   // Lock for the disk, really only used on resize
	allocSetLock      *sync.RWMutex   // Lock for the allocation set
	multiplier        uint            // Multiplier for disk expansion
	increaseThreshold float32         // Threshold for disk expansion
	increasingSize    int32           // Atomic flag to track disk expansion status
}

// Open opens a low level disk (part of virtual disk imp).  Will create a new file if the provided does not exist
func Open(path string, flag int, perm os.FileMode, blockSize uint, blocks uint64, multiplier uint, increaseThreshold float32) (*Disk, error) {
	var err error

	if blockSize == 0 {
		blockSize = 4096 // Default to 4KB blocks
	}

	// Initialize all block locks upfront
	blockLocks := make([]*sync.RWMutex, blocks)
	for i := range blockLocks {
		blockLocks[i] = &sync.RWMutex{}
	}

	disk := &Disk{
		lock:              &sync.RWMutex{},
		allocSetLock:      &sync.RWMutex{},
		blockLocks:        blockLocks,
		multiplier:        multiplier,
		increaseThreshold: increaseThreshold,
		increasingSize:    0,
		header: Header{
			signature:   Signature,
			version:     1,
			blockSize:   uint32(blockSize),
			totalBlocks: blocks,
		},
	}

	// Calculate allocation set size (in bytes)
	allocSetSize := (blocks + 7) / 8 // Round up to nearest byte
	disk.header.allocSetSize = allocSetSize
	disk.header.allocSetOffset = uint64(HeaderSize)
	disk.header.dataBlocksOffset = disk.header.allocSetOffset + alignToBlockSize(allocSetSize, uint64(blockSize))

	// Open the block file
	disk.file, err = os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}

	// Check if the file is empty/new
	fi, err := disk.file.Stat()
	if err != nil {
		return nil, err
	}

	if fi.Size() == 0 {
		// New file - initialize header and allocation set
		err = disk.initializeNewDisk()
		if err != nil {
			return nil, err
		}
	} else {
		// Existing file - read and validate header
		err = disk.readHeader()
		if err != nil {
			return nil, err
		}

		// Update our in-memory state based on the header
		disk.updateFromHeader()
	}

	return disk, nil
}

// Close closes the disk file
func (d *Disk) Close() error {
	if d.file != nil {
		return d.file.Close()
	}
	return nil
}

// alignToBlockSize ensures a value is aligned to block boundaries
func alignToBlockSize(value uint64, blockSize uint64) uint64 {
	remainder := value % blockSize
	if remainder == 0 {
		return value
	}
	return value + (blockSize - remainder)
}

// initializeNewDisk initializes a new disk file with header and allocation set
func (d *Disk) initializeNewDisk() error {
	// Write the header
	err := d.writeHeader()
	if err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Initialize the allocation set (all zeros = all blocks free)
	allocSet := make([]byte, d.header.allocSetSize)
	_, err = d.file.WriteAt(allocSet, int64(d.header.allocSetOffset))
	if err != nil {
		return fmt.Errorf("failed to initialize allocation set: %w", err)
	}

	// Write empty blocks for the data area
	emptyBlock := make([]byte, d.header.blockSize)
	for i := uint64(0); i < d.header.totalBlocks; i++ {
		offset := d.blockToOffset(i)
		_, err = d.file.WriteAt(emptyBlock, offset)
		if err != nil {
			return fmt.Errorf("failed to initialize block %d: %w", i, err)
		}
	}

	// Force sync to ensure all data is written
	err = d.file.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

// writeHeader writes the header to the disk file
func (d *Disk) writeHeader() error {
	buffer := make([]byte, HeaderSize)

	// Write fields to buffer
	binary.LittleEndian.PutUint32(buffer[0:], d.header.signature)
	binary.LittleEndian.PutUint16(buffer[4:], d.header.version)
	binary.LittleEndian.PutUint32(buffer[6:], d.header.blockSize)
	binary.LittleEndian.PutUint64(buffer[10:], d.header.totalBlocks)
	binary.LittleEndian.PutUint64(buffer[18:], d.header.allocSetOffset)
	binary.LittleEndian.PutUint64(buffer[26:], d.header.allocSetSize)
	binary.LittleEndian.PutUint64(buffer[34:], d.header.dataBlocksOffset)

	// Write buffer to file
	_, err := d.file.WriteAt(buffer, 0)
	return err
}

// blockToOffset converts a block number to a file offset
func (d *Disk) blockToOffset(blockNum uint64) int64 {
	return int64(d.header.dataBlocksOffset + (blockNum * uint64(d.header.blockSize)))
}

// readHeader reads the header from the file
func (d *Disk) readHeader() error {
	buffer := make([]byte, HeaderSize)

	// Read header from file
	_, err := d.file.ReadAt(buffer, 0)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	// Parse header values
	signature := binary.LittleEndian.Uint32(buffer[0:])
	if signature != Signature {
		return fmt.Errorf("invalid disk signature: %x", signature)
	}

	d.header.signature = signature
	d.header.version = binary.LittleEndian.Uint16(buffer[4:])
	d.header.blockSize = binary.LittleEndian.Uint32(buffer[6:])
	d.header.totalBlocks = binary.LittleEndian.Uint64(buffer[10:])
	d.header.allocSetOffset = binary.LittleEndian.Uint64(buffer[18:])
	d.header.allocSetSize = binary.LittleEndian.Uint64(buffer[26:])
	d.header.dataBlocksOffset = binary.LittleEndian.Uint64(buffer[34:])

	return nil
}

// updateFromHeader updates the in-memory state based on the header
func (d *Disk) updateFromHeader() {
	// Ensure we have enough block locks
	if uint64(len(d.blockLocks)) < d.header.totalBlocks {
		newBlockLocks := make([]*sync.RWMutex, d.header.totalBlocks)
		copy(newBlockLocks, d.blockLocks)

		// Initialize new locks
		for i := uint64(len(d.blockLocks)); i < d.header.totalBlocks; i++ {
			newBlockLocks[i] = &sync.RWMutex{}
		}

		d.blockLocks = newBlockLocks
	}

}
