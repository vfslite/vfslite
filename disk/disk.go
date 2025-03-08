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
	"sync/atomic"
	"time"
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

// readAllocationSet reads the entire allocation set from the file
func (d *Disk) readAllocationSet() ([]byte, error) {
	// Acquire read lock for allocation set
	d.allocSetLock.RLock()
	defer d.allocSetLock.RUnlock()

	return d.readAllocationSetNoLock()
}

// readAllocationSetNoLock reads the allocation set without acquiring a lock
func (d *Disk) readAllocationSetNoLock() ([]byte, error) {
	// Read the entire allocation set
	buffer := make([]byte, d.header.allocSetSize)
	_, err := d.file.ReadAt(buffer, int64(d.header.allocSetOffset))
	if err != nil {
		return nil, fmt.Errorf("failed to read allocation set: %w", err)
	}

	return buffer, nil
}

// isBlockAllocated checks if a specific block is allocated
func (d *Disk) isBlockAllocated(allocSet []byte, blockNum uint64) bool {
	byteIndex := blockNum / 8
	bitIndex := blockNum % 8

	if byteIndex >= d.header.allocSetSize {
		return false
	}

	return (allocSet[byteIndex] & (1 << bitIndex)) != 0
}

// setBlockAllocation sets or clears a specific block's allocation bit
func (d *Disk) setBlockAllocation(allocSet []byte, blockNum uint64, allocated bool) {
	byteIndex := blockNum / 8
	bitIndex := blockNum % 8

	if byteIndex >= d.header.allocSetSize {
		return
	}

	if allocated {
		// Set the bit
		allocSet[byteIndex] |= 1 << bitIndex
	} else {
		// Clear the bit
		allocSet[byteIndex] &= ^(1 << bitIndex)
	}
}

// updateAllocationSet atomically updates a specific block's allocation status
func (d *Disk) updateAllocationSet(blockNum uint64, status uint8) error {
	// Acquire write lock for allocation set
	d.allocSetLock.Lock()
	defer d.allocSetLock.Unlock()

	// Read current allocation set
	allocSet, err := d.readAllocationSetNoLock()
	if err != nil {
		return err
	}

	// Ensure block number is valid
	byteIndex := blockNum / 8
	if byteIndex >= d.header.allocSetSize {
		return fmt.Errorf("block number %d out of range for allocation set", blockNum)
	}

	// Update the allocation status
	if status == 1 {
		d.setBlockAllocation(allocSet, blockNum, true)
	} else {
		d.setBlockAllocation(allocSet, blockNum, false)
	}

	// Write updated allocation set back to file
	_, err = d.file.WriteAt(allocSet, int64(d.header.allocSetOffset))
	return err
}

// countAllocatedBlocks counts the number of allocated blocks in the allocation set
func (d *Disk) countAllocatedBlocks(allocSet []byte) uint64 {
	var count uint64

	for i := uint64(0); i < d.header.totalBlocks; i++ {
		if d.isBlockAllocated(allocSet, i) {
			count++
		}
	}

	return count
}

// findAndReserveBlock finds an available block and atomically reserves it
func (d *Disk) findAndReserveBlock() (uint64, error) {
	// Acquire write lock for allocation set to ensure atomic operation
	d.allocSetLock.Lock()
	defer d.allocSetLock.Unlock()

	// Read the current allocation set directly from file
	allocSet, err := d.readAllocationSetNoLock()
	if err != nil {
		return 0, err
	}

	// Find the first block with value 0 (unallocated)
	for blockNum := uint64(0); blockNum < d.header.totalBlocks; blockNum++ {
		if !d.isBlockAllocated(allocSet, blockNum) {
			// Found an available block, mark it as allocated
			d.setBlockAllocation(allocSet, blockNum, true)

			// Write the updated allocation set back to file immediately
			_, err = d.file.WriteAt(allocSet, int64(d.header.allocSetOffset))
			if err != nil {
				return 0, err
			}

			// Force sync to ensure the allocation is persisted
			err = d.file.Sync()
			if err != nil {
				// If sync fails, try to mark the block as free again to avoid corruption
				d.setBlockAllocation(allocSet, blockNum, false)
				return 0, fmt.Errorf("failed to persist allocation: %w", err)
			}

			// Return the block number
			return blockNum, nil
		}
	}

	// No available blocks found
	return 0, fmt.Errorf("no available blocks")
}

// nextBlock returns the next block number to write to
func (d *Disk) nextBlock() (uint64, error) {
	const maxRetries = 10
	var retryDelay = 5 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Try to find and reserve a block
		blockNum, err := d.findAndReserveBlock()
		if err == nil {
			return blockNum, nil
		}

		// Check if we need to increase disk size
		// First check if another goroutine is already increasing the size
		if atomic.LoadInt32(&d.increasingSize) == 1 {
			// Another goroutine is handling disk expansion, wait briefly and retry
			time.Sleep(retryDelay)
			retryDelay = time.Duration(float64(retryDelay) * 1.5) // Exponential backoff
			continue
		}

		// Read allocation set to check current usage
		allocSet, err := d.readAllocationSet()
		if err != nil {
			return 0, err
		}

		allocatedBlocks := d.countAllocatedBlocks(allocSet)
		usageRatio := float32(allocatedBlocks) / float32(d.header.totalBlocks)

		if usageRatio >= d.increaseThreshold {
			// Try to set the increasingSize flag
			if atomic.CompareAndSwapInt32(&d.increasingSize, 0, 1) {
				// We're responsible for increasing the disk size
				err := d.IncreaseSize()
				atomic.StoreInt32(&d.increasingSize, 0) // Reset flag when done

				if err != nil {
					return 0, fmt.Errorf("failed to increase disk size: %w", err)
				}

				// The disk has been expanded, try to find a free block again
				continue
			} else {
				// Another goroutine is handling the expansion, wait and retry
				time.Sleep(retryDelay)
				retryDelay = time.Duration(float64(retryDelay) * 1.5) // Exponential backoff
				continue
			}
		}

		// If we get here, no blocks are available, and we don't need to expand
		// Wait a bit and retry once more
		if attempt < maxRetries-1 {
			time.Sleep(retryDelay)
			continue
		}
	}

	// After all retries, if we still couldn't get a block
	return 0, fmt.Errorf("no available blocks after multiple retries")
}

// IncreaseSize increases the size of the disk
func (d *Disk) IncreaseSize() error {
	// Acquire write locks
	d.lock.Lock()
	defer d.lock.Unlock()

	d.allocSetLock.Lock()
	defer d.allocSetLock.Unlock()

	// Calculate new size
	oldBlocks := d.header.totalBlocks
	newBlocks := oldBlocks * uint64(d.multiplier)

	// Calculate new allocation set size
	newAllocSetSize := (newBlocks + 7) / 8 // Round up to nearest byte

	// Read current allocation set
	currentAllocSet, err := d.readAllocationSetNoLock()
	if err != nil {
		return fmt.Errorf("failed to read allocation set: %w", err)
	}

	// Create new allocation set
	newAllocSet := make([]byte, newAllocSetSize)
	copy(newAllocSet, currentAllocSet)

	// If the allocation set needs to be moved due to size increase
	if newAllocSetSize > d.header.allocSetSize {
		// Update header
		d.header.allocSetSize = newAllocSetSize

		// The data blocks may need to be moved if the allocation set grew
		oldDataBlocksOffset := d.header.dataBlocksOffset
		d.header.dataBlocksOffset = d.header.allocSetOffset + alignToBlockSize(newAllocSetSize, uint64(d.header.blockSize))

		// If the data blocks need to be moved
		if d.header.dataBlocksOffset > oldDataBlocksOffset {
			// This is a complex operation - we need to move all data blocks
			// We'll do this from back to front to avoid overwriting data

			for blockNum := oldBlocks - 1; blockNum >= 0; blockNum-- {
				// Calculate old and new offsets
				oldOffset := int64(oldDataBlocksOffset + (blockNum * uint64(d.header.blockSize)))
				newOffset := d.blockToOffset(blockNum) // Uses the updated header

				// Read the block data
				blockData := make([]byte, d.header.blockSize)
				_, err := d.file.ReadAt(blockData, oldOffset)
				if err != nil {
					return fmt.Errorf("failed to read block %d during resize: %w", blockNum, err)
				}

				// Write to new location
				_, err = d.file.WriteAt(blockData, newOffset)
				if err != nil {
					return fmt.Errorf("failed to write block %d during resize: %w", blockNum, err)
				}

				// If we hit block 0, we're done
				if blockNum == 0 {
					break
				}
			}
		}
	}

	// Write the updated allocation set
	_, err = d.file.WriteAt(newAllocSet, int64(d.header.allocSetOffset))
	if err != nil {
		return fmt.Errorf("failed to write updated allocation set: %w", err)
	}

	// Extend the file with new empty blocks
	emptyBlock := make([]byte, d.header.blockSize)
	for i := oldBlocks; i < newBlocks; i++ {
		offset := d.blockToOffset(i)
		_, err = d.file.WriteAt(emptyBlock, offset)
		if err != nil {
			return fmt.Errorf("failed to initialize new block %d: %w", i, err)
		}
	}

	// Update the header with new total blocks
	d.header.totalBlocks = newBlocks

	// Write updated header
	err = d.writeHeader()
	if err != nil {
		return fmt.Errorf("failed to update header after resize: %w", err)
	}

	// Ensure changes are persisted
	err = d.file.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync after resize: %w", err)
	}

	// Expand block locks slice
	newBlockLocks := make([]*sync.RWMutex, newBlocks)
	copy(newBlockLocks, d.blockLocks)

	// Initialize new locks
	for i := oldBlocks; i < newBlocks; i++ {
		newBlockLocks[i] = &sync.RWMutex{}
	}

	d.blockLocks = newBlockLocks

	return nil
}

// Write writes data to a block and returns the block number
func (d *Disk) Write(chunk []byte) (uint64, error) {
	// Check chunk size first
	if uint(len(chunk)) > uint(d.header.blockSize) {
		return 0, fmt.Errorf("chunk size %d exceeds block size %d", len(chunk), d.header.blockSize)
	}

	// Use retry loop to handle potential temporary issues
	const maxRetries = 5
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Find and reserve the next available block
		blockNum, err := d.nextBlock()
		if err != nil {
			lastErr = err
			// If it's a retriable error, wait and continue
			if err.Error() == "no available blocks" ||
				err.Error() == "no available blocks after multiple retries" {
				time.Sleep(time.Duration(10*(attempt+1)) * time.Millisecond)
				continue
			}
			return 0, err
		}

		// Write the data to the block (without updating allocation set again)
		err = d.writeAtWithoutAllocationUpdate(chunk, blockNum)
		if err != nil {
			// If write fails, try to mark the block as free again
			_ = d.updateAllocationSet(blockNum, 0)
			lastErr = err
			continue
		}

		// Success
		return blockNum, nil
	}

	// If we get here, all attempts failed
	return 0, fmt.Errorf("failed to write data after multiple attempts: %w", lastErr)
}

// writeAtWithoutAllocationUpdate writes data to a block without updating the allocation set
func (d *Disk) writeAtWithoutAllocationUpdate(chunk []byte, block uint64) error {
	// Validate block number
	if block >= d.header.totalBlocks {
		return fmt.Errorf("block number %d out of range (max: %d)", block, d.header.totalBlocks-1)
	}

	// Acquire lock for the specific block
	if int(block) < len(d.blockLocks) && d.blockLocks[block] != nil {
		d.blockLocks[block].Lock()
		defer d.blockLocks[block].Unlock()
	} else {
		return fmt.Errorf("block lock not initialized for block %d", block)
	}

	// Calculate offset for the block
	offset := d.blockToOffset(block)

	// Pad the chunk if needed
	paddedChunk := chunk
	if uint(len(chunk)) < uint(d.header.blockSize) {
		paddedChunk = make([]byte, d.header.blockSize)
		copy(paddedChunk, chunk)
	}

	// Write to the file
	_, err := d.file.WriteAt(paddedChunk, offset)
	return err
}

// WriteAt writes data to a block at a specific block number
func (d *Disk) WriteAt(chunk []byte, block uint64) error {
	// Validate block number
	if block >= d.header.totalBlocks {
		return fmt.Errorf("block number %d out of range (max: %d)", block, d.header.totalBlocks-1)
	}

	// Check chunk size
	if uint(len(chunk)) > uint(d.header.blockSize) {
		return fmt.Errorf("chunk size %d exceeds block size %d", len(chunk), d.header.blockSize)
	}

	// Acquire lock for the specific block
	if int(block) < len(d.blockLocks) && d.blockLocks[block] != nil {
		d.blockLocks[block].Lock()
		defer d.blockLocks[block].Unlock()
	} else {
		return fmt.Errorf("block lock not initialized for block %d", block)
	}

	// Calculate offset for the block
	offset := d.blockToOffset(block)

	// Pad the chunk if needed
	paddedChunk := chunk
	if uint(len(chunk)) < uint(d.header.blockSize) {
		paddedChunk = make([]byte, d.header.blockSize)
		copy(paddedChunk, chunk)
	}

	// Write to the file
	_, err := d.file.WriteAt(paddedChunk, offset)
	if err != nil {
		return err
	}

	// Update allocation set to mark this block as used
	return d.updateAllocationSet(block, 1)
}

// ReadAt reads data from a specific block
func (d *Disk) ReadAt(block uint64) ([]byte, error) {
	// Validate block number
	if block >= d.header.totalBlocks {
		return nil, fmt.Errorf("block number %d out of range (max: %d)", block, d.header.totalBlocks-1)
	}

	// Acquire read lock for the specific block
	if int(block) < len(d.blockLocks) && d.blockLocks[block] != nil {
		d.blockLocks[block].RLock()
		defer d.blockLocks[block].RUnlock()
	} else {
		return nil, fmt.Errorf("block lock not initialized for block %d", block)
	}

	// Calculate offset for the block
	offset := d.blockToOffset(block)

	// Read data from the block
	data := make([]byte, d.header.blockSize)
	_, err := d.file.ReadAt(data, offset)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Delete marks a block as free in the allocation set
func (d *Disk) Delete(block uint64) error {
	// Validate block number
	if block >= d.header.totalBlocks {
		return fmt.Errorf("block number %d out of range (max: %d)", block, d.header.totalBlocks-1)
	}

	// Acquire write lock for the specific block
	if int(block) < len(d.blockLocks) && d.blockLocks[block] != nil {
		d.blockLocks[block].Lock()
		defer d.blockLocks[block].Unlock()
	} else {
		return fmt.Errorf("block lock not initialized for block %d", block)
	}

	// Update allocation set to mark this block as free (0)
	return d.updateAllocationSet(block, 0)
}
