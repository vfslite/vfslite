<div>
    <h1 align="left"><img width="256" src="artwork/vfsl_logo.png"></h1>
</div>

VFSLite is a lightweight, single file virtual file system.  VFSLite can be accessed via Go or shared C library.

## Features
- **Block-Based Filesystem** - VFSLite is a block-based filesystem.  This means that files are stored in used-defined fixed-size blocks.
- **Single File** - VFSLite is a single file, the entire filesystem is stored in a single file.
- **Metadata Management** - Custom metadeta can be stored with each file.  This allows for easy storage and retrieval of file metadata.
- **Go and C API** - VFSLite can be accessed via Go or shared C library.
- **Block Locking** - VFSLite supports granular block RW locking.
- **Concurrent Safe** - VFSLite is designed to be concurrent safe.  Multiple goroutines can read and write to the filesystem simultaneously.
- **Hierarchical Structure** - VFSLite supports a hierarchical structure.  This means that you can create directories and subdirectories to organize your files.
- **Dynamic Disk Expansion** - VFSLite supports automatic dynamic disk expansion.
- **Stream I/O** - VFSLite supports stream I/O.  This means that you can read and write files in a streaming fashion.

## Discord Community
Join the VFSLite Discord community to work on development, ask questions, make requests, and more.
[![Discord](https://img.shields.io/discord/1348106697234841712?color=0970da&label=Discord&logo=discord&logoColor=white)](https://discord.gg/J7z7nNMBRX)

## GO API

### Import
```go
import "github.com/vfslite/vfslite"
```

### Opening a Virtual Disk
To create or open a virtual disk, use the `Open` function. The following example opens (or creates) a disk file with a specified block size and an initial number of blocks:
```go
vfs, err := vfslite.Open("disk.vfs", 4096, 100)
if err != nil {
    // handle error appropriately
}
defer vfs.Close()
```

### Creating Files and Directories
The root block is the entry point of your virtual filesystem.
```go
rootBlock := vfs.GetRootBlock()
```

#### Creating directory
```go
dirBlock, err := vfs.CreateDirectoryBlock(rootBlock, "myDirectory", nil)
if err != nil {
    // handle error
}
```

#### Creating file
```go
fileBlock, err := vfs.CreateFileBlock(rootBlock, "myFile.txt", nil)
if err != nil {
    // handle error
}
```

### Reading and Writing Files
VFSLite supports two primary methods for file operations.

#### Writing Files
**For small files** Use the `WriteFile` function which writes an entire file at once.
```go
data := []byte("Hello, VFSLite!")

fileBlock, err := vfs.WriteFile(rootBlock, "myFile.txt", data)
if err != nil {
    // handle error
}
```

**For large files** Use a `StreamWriter` to write data in chunks.
```go
writer, err := vfs.CreateStreamWriter(rootBlock, "largeFile.txt")
if err != nil {
    // handle error
}

_, err = writer.Write(largeData)
if err != nil {
    // handle error
}

err = writer.Close()
if err != nil {
    // handle error
}
```

#### Reading Files
**For small files** Use the `ReadFile` function which reads an entire file at once.
```go
data, err := vfs.ReadFile(fileBlock)
if err != nil {
    // handle error
}

fmt.Println(string(data))
```

**For large files** Use a `StreamReader` to read data in chunks.
```go
reader, err := vfs.OpenStreamReader(fileBlock)
if err != nil {
    // handle error
}

defer reader.Close()

buffer := make([]byte, 1024)
n, err := reader.Read(buffer)
if err != nil && err != io.EOF {
    // handle error
}

fmt.Println(string(buffer[:n]))
```

### Metadata Management
VFSLite supports custom metadata for files and directories.  Metadata is stored as key-value pairs.


#### Setting Extra Metadata
```go
err = vfs.SetExtraMetadata(fileBlock, "author", "Jane Doe")
if err != nil {
    // handle error
}
```

#### Getting Extra Metadata
```go
value, exists, err := vfs.GetExtraMetadata(fileBlock, "author")
if err != nil {
    // handle error
}

if exists {
    fmt.Println("Author:", value)
}
```

### Directory Navigation
VFSLite supports hierarchical directory structure.  You can navigate directories.

#### Listing Children
```go
children, err := vfs.ListChildren(rootBlock)
if err != nil {
    // handle error
}

for _, child := range children {
    // Process child block
}
```

#### Finding Directories and Files by Path
```go
// Retrieve a directory by path
dirBlock, err := vfs.GetDirectory(rootBlock, "myDirectory")
if err != nil {
    // handle error
}

// Retrieve a file by path
fileBlock, err = vfs.GetFileByPath(rootBlock, "myDirectory/myFile.txt")
if err != nil {
    // handle error
}
```

### Deleting Files and Directories
VFSLite supports several methods for deleting files and directories.

#### Deleting Files
```go
// Delete a file by block ID (removes file and all its data blocks)
err = vfs.DeleteFile(fileBlock)
if err != nil {
    // handle error
}

// Delete a file by name from a parent directory
err = vfs.DeleteFileFromParent(dirBlock, "myFile.txt")
if err != nil {
    // handle error
}
```

#### Deleting Directories
```go
// Delete a directory by block ID (recursively removes all contents)
err = vfs.DeleteDirectory(dirBlock)
if err != nil {
    // handle error
}

// Delete a directory by name from a parent directory
err = vfs.DeleteDirectoryFromParent(parentDir, "myDirectory")
if err != nil {
    // handle error
}
```

#### Deleting by Path
```go
// Delete a file or directory by path
err = vfs.DeleteByPath(rootBlock, "myDirectory/myFile.txt")
if err != nil {
    // handle error
}

// Delete a directory and all its contents by path
err = vfs.DeleteByPath(rootBlock, "myDirectory")
if err != nil {
    // handle error
}
```

### Renaming Files and Directories
VFSLite supports renaming files and directories.

#### Renaming Files
```go
// Rename a file in a directory
err = vfs.RenameFile(dirBlock, "oldFileName.txt", "newFileName.txt")
if err != nil {
    // handle error
}
```

#### Renaming Directories
```go
// Rename a directory in a parent directory
err = vfs.RenameDirectory(parentDir, "oldDirName", "newDirName")
if err != nil {
    // handle error
}
```

#### Renaming by Path
```go
// Rename a file or directory using a path
err = vfs.RenameByPath(rootBlock, "myDirectory/oldFile.txt", "newFile.txt")
if err != nil {
    // handle error
}

// Rename a directory using a path
err = vfs.RenameByPath(rootBlock, "myDirectory/oldSubDir", "newSubDir")
if err != nil {
    // handle error
}
```


### Updating Files

#### Updating a file by block ID
```go
newData := []byte("Updated content for the file")
err = vfs.UpdateFile(fileBlock, newData)
if err != nil {
    // handle error
}
```

#### Updating a file by path
```go
newData := []byte("Updated content for the file")
err = vfs.UpdateFileByPath(rootBlock, "myDirectory/myFile.txt", newData)
if err != nil {
    // handle error
}
```

### Closing
Closing a VFSLite instance.
```go
err = vfs.Close()
if err != nil {
    // handle error
}
```

### Getting File Size
```go
size, err := vfs.GetFileSize(fileBlock)
if err != nil {
    // handle error
}

fmt.Printf("File size: %d bytes\n", size)
```

### Updating Block Metadata
```go
// Update metadata using a function that modifies the metadata object
err = vfs.UpdateBlockMetadata(fileBlock, func(meta *vfslite.Metadata) {
    meta.Extra["lastAccessed"] = time.Now().Format(time.RFC3339)
    meta.Extra["version"] = "1.2"
})
if err != nil {
    // handle error
}
```

### Appending Data to Files
```go
additionalData := []byte("More content to add to the file")

err = vfs.AppendData(fileBlock, additionalData)
if err != nil {
    // handle error
}
```

## C API
Firstly you must build and include the shared library.  This is important :)

### Opening and closing VFSLite
```c
/* Open a virtual disk (creates if it doesn't exist) */
int vfsl_open(const char* diskName, unsigned int blockSize, unsigned long long initialBlocks);

/* Close a VFSLite instance */
int vfsl_close(int handle);
```

### Basic Navigation
```c
/* Get the root block of the filesystem */
unsigned long long vfsl_get_root_block(int handle);

/* Get directory by path */
unsigned long long vfsl_get_directory(int handle, unsigned long long startDir, const char* path);

/* Get file by name from a directory */
unsigned long long vfsl_get_file(int handle, unsigned long long dirBlock, const char* fileName);

/* Get file by path */
unsigned long long vfsl_get_file_by_path(int handle, unsigned long long startDir, const char* path);

/* List children of a directory */
size_t vfsl_list_children(int handle, unsigned long long blockNum, void* buffer, size_t bufferSize);
```

### Creating Files and Directories
```c
/* Create a directory */
unsigned long long vfsl_create_directory_block(int handle, unsigned long long parentBlock, const char* dirName);

/* Create a file */
unsigned long long vfsl_create_file_block(int handle, unsigned long long parentBlock, const char* fileName);
```

### File Operations
```c
/* Write a file in one operation */
unsigned long long vfsl_write_file(int handle, unsigned long long parentDir, const char* fileName,
const void* data, size_t dataSize);

/* Read a file in one operation */
size_t vfsl_read_file(int handle, unsigned long long fileBlock, void* buffer, size_t bufferSize);

/* Get file size */
unsigned long long vfsl_get_file_size(int handle, unsigned long long fileBlock);

/* Update file content */
int vfsl_update_file(int handle, unsigned long long fileBlock, const void* data, size_t dataSize);

/* Update file by path */
int vfsl_update_file_by_path(int handle, unsigned long long startDir, const char* path,
const void* data, size_t dataSize);
```

### Streaming I/O
```c
/* Create a stream writer */
int vfsl_create_stream_writer(int handle, unsigned long long parentBlock, const char* fileName);

/* Write to a stream */
int vfsl_stream_writer_write(int writerHandle, const void* data, size_t dataSize);

/* Close a stream writer */
int vfsl_stream_writer_close(int writerHandle);

/* Open a stream reader */
int vfsl_open_stream_reader(int handle, unsigned long long fileBlock);

/* Read from a stream */
int vfsl_stream_reader_read(int readerHandle, void* buffer, size_t bufferSize);

/* Close a stream reader */
int vfsl_stream_reader_close(int readerHandle);
```

### Deletion Operations
```c
/* Delete a file by block ID */
int vfsl_delete_file(int handle, unsigned long long fileBlock);

/* Delete a file from a parent directory */
int vfsl_delete_file_from_parent(int handle, unsigned long long parentDir, const char* fileName);

/* Delete a directory by block ID */
int vfsl_delete_directory(int handle, unsigned long long dirBlock);

/* Delete a directory from a parent */
int vfsl_delete_directory_from_parent(int handle, unsigned long long parentDir, const char* dirName);

/* Delete by path */
int vfsl_delete_by_path(int handle, unsigned long long startDir, const char* path);
```

### Renaming Operations
```c
/* Rename a file */
int vfsl_rename_file(int handle, unsigned long long dirBlock, const char* oldName, const char* newName);

/* Rename a directory */
int vfsl_rename_directory(int handle, unsigned long long parentDir, const char* oldName, const char* newName);

/* Rename by path */
int vfsl_rename_by_path(int handle, unsigned long long startDir, const char* path, const char* newName);
```

### Metadata Operations
```c
/* Get block type */
int vfsl_get_block_type(int handle, unsigned long long blockNum);

/* Get block data */
size_t vfsl_get_block_data(int handle, unsigned long long blockNum, void* buffer, size_t bufferSize);

/* Get block metadata */
size_t vfsl_get_block_metadata(int handle, unsigned long long blockNum, void* buffer, size_t bufferSize);

/* Set extra metadata */
int vfsl_set_extra_metadata(int handle, unsigned long long blockNum, const char* key, const char* value);

/* Get extra metadata */
int vfsl_get_extra_metadata(int handle, unsigned long long blockNum, const char* key,
void* buffer, size_t bufferSize);

/* Get file size */
unsigned long long vfsl_get_file_size(int handle, unsigned long long fileBlock);
```

### Hierarchical Operations
```c
/* Add a child block to a parent */
int vfsl_add_child(int handle, unsigned long long parentBlock, unsigned long long childBlock);

/* Remove a child block from a parent */
int vfsl_remove_child(int handle, unsigned long long parentBlock, unsigned long long childBlock);
```

### Additional Operations
```c
/* Append data to an existing file */
int vfsl_append_data(int handle, unsigned long long fileBlock, const void* data, size_t dataSize);

/* Get block header information */
size_t vfsl_get_block_header(int handle, unsigned long long blockNum, void* buffer, size_t bufferSize);

/* Update block metadata with JSON */
int vfsl_update_block_metadata(int handle, unsigned long long blockNum, const char* metadataJSON);

/* Rename a block (change its name) */
int vfsl_rename_block(int handle, unsigned long long blockNum, const char* newName);
```