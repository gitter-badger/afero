// Copyright © 2014 Steve Francia <spf@spf13.com>.
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

package afero

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var mux = &sync.Mutex{}

type MemMapFs struct {
	data  map[string]File
	mutex *sync.RWMutex
}

func (m *MemMapFs) lock() {
	mx := m.getMutex()
	mx.Lock()
}
func (m *MemMapFs) unlock()  { m.getMutex().Unlock() }
func (m *MemMapFs) rlock()   { m.getMutex().RLock() }
func (m *MemMapFs) runlock() { m.getMutex().RUnlock() }

func (m *MemMapFs) getData() map[string]File {
	if m.data == nil {
		m.data = make(map[string]File)
	}
	return m.data
}

func (m *MemMapFs) getMutex() *sync.RWMutex {
	mux.Lock()
	if m.mutex == nil {
		m.mutex = &sync.RWMutex{}
	}
	mux.Unlock()
	return m.mutex
}

type MemDirMap map[string]File

func (m MemDirMap) Len() int      { return len(m) }
func (m MemDirMap) Add(f File)    { m[f.Name()] = f }
func (m MemDirMap) Remove(f File) { delete(m, f.Name()) }
func (m MemDirMap) Files() (files []File) {
	for _, f := range m {
		files = append(files, f)
	}
	sort.Sort(filesSorter(files))
	return files
}

type filesSorter []File

// implement sort.Interface for []File
func (s filesSorter) Len() int           { return len(s) }
func (s filesSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s filesSorter) Less(i, j int) bool { return s[i].Name() < s[j].Name() }

func (m MemDirMap) Names() (names []string) {
	for x := range m {
		names = append(names, x)
	}
	return names
}

func (MemMapFs) Name() string { return "MemMapFS" }

func (m *MemMapFs) Create(name string) (File, error) {
	name = abs(name)
	m.lock()
	file := MemFileCreate(name)
	m.getData()[name] = file
	m.registerWithParent(file)
	m.unlock()
	return file, nil
}

func (m *MemMapFs) unRegisterWithParent(fileName string) {
	f, err := m.lockfreeOpen(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("Open err:", err)
		}
		return
	}
	parent := m.findParent(f)
	if parent == nil {
		log.Fatal("parent of ", f.Name(), " is nil")
	}
	pmem := parent.(*InMemoryFile)
	pmem.memDir.Remove(f)
}

func abs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		log.Println("ABS ERROR", err)
		abs = path
	}
	return filepath.Clean(abs)
}

func absParent(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return filepath.Dir(filepath.Clean(abs))
}

func (m *MemMapFs) findParent(f File) File {
	pfile, err := m.lockfreeOpen(absParent(f.Name()))
	if err != nil {
		return nil
	}
	//fmt.Println("absParent1:", f.Name(), ":", pfile.Name())
	return pfile
}

func (m *MemMapFs) registerWithParent(f File) {
	if f == nil {
		return
	}
	parent := m.findParent(f)
	if parent == nil {
		pdir := absParent(f.Name())
		//fmt.Println("absParent2:", f.Name(), ":", pdir)
		err := m.lockfreeMkdir(pdir, 0777)
		if err != nil {
			// should never happen
			log.Println("Mkdir error:", pdir, err)
			return
		}
		parent, err = m.lockfreeOpen(pdir)
		if err != nil {
			// should also never happen
			log.Println("Open after Mkdir error:", err)
			return
		}
	}
	if parent.Name() != f.Name() {
		pmem := parent.(*InMemoryFile)
		if pmem.memDir == nil {
			pmem.memDir = &MemDirMap{}
			m.List()
			log.Fatal("memdir is nil parent:", parent.Name(), " file:", f.Name())
		}
		pmem.memDir.Add(f)
	}
}

func (m *MemMapFs) lockfreeMkdir(name string, perm os.FileMode) error {
	_, ok := m.getData()[name]
	if ok {
		return ErrFileExists
	} else {
		item := &InMemoryFile{name: name, memDir: &MemDirMap{}, dir: true}
		m.getData()[name] = item
		m.registerWithParent(item)
	}
	return nil
}

func (m *MemMapFs) Mkdir(name string, perm os.FileMode) error {
	name = abs(name)
	m.rlock()
	_, ok := m.getData()[name]
	m.runlock()
	if ok {
		return ErrFileExists
	} else {
		m.lock()
		item := &InMemoryFile{name: name, memDir: &MemDirMap{}, dir: true}
		m.getData()[name] = item
		m.registerWithParent(item)
		m.unlock()
	}
	return nil
}

func (m *MemMapFs) MkdirAll(name string, perm os.FileMode) error {
	name = abs(name)
	return m.Mkdir(name, 0777)
}

func (m *MemMapFs) Open(name string) (File, error) {
	name = abs(name)
	m.rlock()
	f, ok := m.getData()[name]
	ff, ok := f.(*InMemoryFile)
	if ok {
		ff.Open()
	}
	m.runlock()

	if ok {
		return f, nil
	} else {
		return nil, ErrFileNotFound
	}
}

func (m *MemMapFs) lockfreeOpen(name string) (File, error) {
	f, ok := m.getData()[name]
	ff, ok := f.(*InMemoryFile)
	if ok {
		ff.Open()
	}
	if ok {
		return f, nil
	} else {
		return nil, ErrFileNotFound
	}
}

func (m *MemMapFs) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	name = abs(name)
	file, err := m.Open(name)
	if os.IsNotExist(err) && (flag&os.O_CREATE > 0) {
		file, err = m.Create(name)
	}
	if err != nil {
		return nil, err
	}
	if flag&os.O_APPEND > 0 {
		_, err = file.Seek(0, os.SEEK_END)
		if err != nil {
			file.Close()
			return nil, err
		}
	}
	if flag&os.O_TRUNC > 0 && flag&(os.O_RDWR|os.O_WRONLY) > 0 {
		err = file.Truncate(0)
		if err != nil {
			file.Close()
			return nil, err
		}
	}
	return file, nil
}

func (m *MemMapFs) Remove(name string) error {
	name = abs(name)
	m.lock()
	defer m.unlock()

	if _, ok := m.getData()[name]; ok {
		m.unRegisterWithParent(name)
		delete(m.getData(), name)
	} else {
		return &os.PathError{"remove", name, os.ErrNotExist}
	}
	return nil
}

func (m *MemMapFs) RemoveAll(name string) error {
	name = abs(name)
	m.lock()
	m.unRegisterWithParent(name)
	m.unlock()

	m.rlock()
	defer m.runlock()

	for p, _ := range m.getData() {
		if strings.HasPrefix(p, name) {
			m.runlock()
			m.lock()
			delete(m.getData(), p)
			m.unlock()
			m.rlock()
		}
	}
	return nil
}

func (m *MemMapFs) Rename(oldname, newname string) error {
	oldname = abs(oldname)
	newname = abs(newname)
	m.rlock()
	defer m.runlock()
	if _, ok := m.getData()[oldname]; ok {
		if _, ok := m.getData()[newname]; !ok {
			m.runlock()
			m.lock()
			m.getData()[newname] = m.getData()[oldname]
			delete(m.getData(), oldname)
			m.unlock()
			m.rlock()
		} else {
			return ErrDestinationExists
		}
	} else {
		return ErrFileNotFound
	}
	return nil
}

func (m *MemMapFs) Stat(name string) (os.FileInfo, error) {
	name = abs(name)
	f, err := m.Open(name)
	if err != nil {
		return nil, err
	}
	return &InMemoryFileInfo{file: f.(*InMemoryFile)}, nil
}

func (m *MemMapFs) Chmod(name string, mode os.FileMode) error {
	name = abs(name)
	f, ok := m.getData()[name]
	if !ok {
		return &os.PathError{"chmod", name, ErrFileNotFound}
	}

	ff, ok := f.(*InMemoryFile)
	if ok {
		m.lock()
		ff.mode = mode
		m.unlock()
	} else {
		return errors.New("Unable to Chmod Memory File")
	}
	return nil
}

func (m *MemMapFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	name = abs(name)
	f, ok := m.getData()[name]
	if !ok {
		return &os.PathError{"chtimes", name, ErrFileNotFound}
	}

	ff, ok := f.(*InMemoryFile)
	if ok {
		m.lock()
		ff.modtime = mtime
		m.unlock()
	} else {
		return errors.New("Unable to Chtime Memory File")
	}
	return nil
}

func (m *MemMapFs) List() {
	for _, x := range m.data {
		y, _ := x.Stat()
		fmt.Println(x.Name(), y.Size(), y.IsDir(),
			x.(*InMemoryFile).memDir)
	}
}
