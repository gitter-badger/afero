// Copyright © 2015 Steve Francia <spf@spf13.com>.
// Copyright 2013 tsuru authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mem

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

import "time"

type File struct {
	// atomic requires 64-bit alignment for struct field access
	at           int64
	readDirCount int64

	FileName string
	MemDir   Dir
	Dir      bool
	Mode     os.FileMode
	Modtime  time.Time

	sync.Mutex
	closed bool
	data   []byte
}

func Create(name string) *File {
	return &File{FileName: name, Mode: os.ModeTemporary, Modtime: time.Now()}
}

func (f *File) Open() error {
	atomic.StoreInt64(&f.at, 0)
	atomic.StoreInt64(&f.readDirCount, 0)
	f.Lock()
	f.closed = false
	f.Unlock()
	return nil
}

func (f *File) Close() error {
	f.Lock()
	f.closed = true
	f.Unlock()
	return nil
}

func (f *File) Name() string {
	return f.FileName
}

func (f *File) Stat() (os.FileInfo, error) {
	return &FileInfo{f}, nil
}

func (f *File) Sync() error {
	return nil
}

func (f *File) Readdir(count int) (res []os.FileInfo, err error) {
	var outLength int64

	f.Lock()
	files := f.MemDir.Files()[f.readDirCount:]
	if count > 0 {
		if len(files) < count {
			outLength = int64(len(files))
		} else {
			outLength = int64(count)
		}
		if len(files) == 0 {
			err = io.EOF
		}
	} else {
		outLength = int64(len(files))
	}
	f.readDirCount += outLength
	f.Unlock()

	res = make([]os.FileInfo, outLength)
	for i := range res {
		res[i], _ = files[i].Stat()
	}

	return res, err
}

func (f *File) Readdirnames(n int) (names []string, err error) {
	fi, err := f.Readdir(n)
	names = make([]string, len(fi))
	for i, f := range fi {
		_, names[i] = filepath.Split(f.Name())
	}
	return names, err
}

func (f *File) Read(b []byte) (n int, err error) {
	f.Lock()
	defer f.Unlock()
	if f.closed == true {
		return 0, ErrFileClosed
	}
	if len(b) > 0 && int(f.at) == len(f.data) {
		return 0, io.EOF
	}
	if len(f.data)-int(f.at) >= len(b) {
		n = len(b)
	} else {
		n = len(f.data) - int(f.at)
	}
	copy(b, f.data[f.at:f.at+int64(n)])
	atomic.AddInt64(&f.at, int64(n))
	return
}

func (f *File) ReadAt(b []byte, off int64) (n int, err error) {
	atomic.StoreInt64(&f.at, off)
	return f.Read(b)
}

func (f *File) Truncate(size int64) error {
	if f.closed == true {
		return ErrFileClosed
	}
	if size < 0 {
		return ErrOutOfRange
	}
	if size > int64(len(f.data)) {
		diff := size - int64(len(f.data))
		f.data = append(f.data, bytes.Repeat([]byte{00}, int(diff))...)
	} else {
		f.data = f.data[0:size]
	}
	return nil
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.closed == true {
		return 0, ErrFileClosed
	}
	switch whence {
	case 0:
		atomic.StoreInt64(&f.at, offset)
	case 1:
		atomic.AddInt64(&f.at, int64(offset))
	case 2:
		atomic.StoreInt64(&f.at, int64(len(f.data))+offset)
	}
	return f.at, nil
}

func (f *File) Write(b []byte) (n int, err error) {
	n = len(b)
	cur := atomic.LoadInt64(&f.at)
	f.Lock()
	defer f.Unlock()
	diff := cur - int64(len(f.data))
	var tail []byte
	if n+int(cur) < len(f.data) {
		tail = f.data[n+int(cur):]
	}
	if diff > 0 {
		f.data = append(bytes.Repeat([]byte{00}, int(diff)), b...)
		f.data = append(f.data, tail...)
	} else {
		f.data = append(f.data[:cur], b...)
		f.data = append(f.data, tail...)
	}

	atomic.StoreInt64(&f.at, int64(len(f.data)))
	return
}

func (f *File) WriteAt(b []byte, off int64) (n int, err error) {
	atomic.StoreInt64(&f.at, off)
	return f.Write(b)
}

func (f *File) WriteString(s string) (ret int, err error) {
	return f.Write([]byte(s))
}

func (f *File) Info() *FileInfo {
	return &FileInfo{File: f}
}

type FileInfo struct {
	File *File
}

// Implements os.FileInfo
func (s *FileInfo) Name() string {
	_, name := filepath.Split(s.File.Name())
	return name
}
func (s *FileInfo) Mode() os.FileMode  { return s.File.Mode }
func (s *FileInfo) ModTime() time.Time { return s.File.Modtime }
func (s *FileInfo) IsDir() bool        { return s.File.Dir }
func (s *FileInfo) Sys() interface{}   { return nil }
func (s *FileInfo) Size() int64 {
	if s.IsDir() {
		return int64(42)
	}
	return int64(len(s.File.data))
}

var (
	ErrFileClosed        = errors.New("File is closed")
	ErrOutOfRange        = errors.New("Out of range")
	ErrTooLarge          = errors.New("Too large")
	ErrFileNotFound      = os.ErrNotExist
	ErrFileExists        = os.ErrExist
	ErrDestinationExists = os.ErrExist
)
