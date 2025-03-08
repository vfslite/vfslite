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


### Additional Operations
- **Appending Data** You can use `AppendData` to add extra data blocks to a file.
- **Updating Metadata** You can use `UpdateBlockMetadata` to modify metadata using a custom update function.
- **Getting File Size** You can use `GetFileSize` to calculate the size of a file by summing the sizes of its data blocks.

## C API
..