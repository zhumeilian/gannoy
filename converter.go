package gannoy

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func NewConverter(from string, dim, tree, K int, order binary.ByteOrder) Converter {
	if filepath.Ext(from) == ".csv" {
		return csvConverter{
			dim:   dim,
			tree:  tree,
			K:     K,
			order: order,
		}
	} else {
		return converter{
			dim:   dim,
			tree:  tree,
			K:     K,
			order: order,
		}
	}
}

type Converter interface {
	Convert(string, string, string, string) error
}

type converter struct {
	dim   int
	tree  int
	K     int
	order binary.ByteOrder
}

func (c converter) Convert(from, path, to, mapPath string) error {
	ann, err := os.Open(from)
	if err != nil {
		return err
	}
	defer ann.Close()

	var maps map[int]int
	if mapPath != "" {
		maps, err = c.initializeMaps(mapPath)
		if err != nil {
			return err
		}
	}

	err = CreateMeta(path, to, c.tree, c.dim, c.K)
	if err != nil {
		return err
	}

	gannoy, err := NewGannoyIndex(filepath.Join(path, to+".meta"), Angular{}, RandRandom{})
	if err != nil {
		return err
	}

	stat, _ := ann.Stat()
	count := int(stat.Size() / c.nodeSize())

	keys := make([]int, count)
	vecs := make([][]float64, count)
	for i := 0; i < count; i++ {
		b := make([]byte, c.nodeSize())
		_, err = syscall.Pread(int(ann.Fd()), b, c.offset(i))
		if err != nil {
			return err
		}

		buf := bytes.NewReader(b)

		var nDescendants int32
		binary.Read(buf, c.order, &nDescendants)
		if int(nDescendants) != 1 {
			break
		}

		buf.Seek(int64(4*2), io.SeekCurrent) // skip children

		vec := make([]float64, c.dim)
		binary.Read(buf, c.order, &vec)

		key := i
		if mapPath != "" {
			if k, ok := maps[i]; ok {
				key = k
			} else {
				return fmt.Errorf("Index is not found in mapping file.\n")
			}
		}
		keys[i] = key
		vecs[i] = vec
	}
	return gannoy.AddItems(keys, vecs)
}

func (c converter) offset(index int) int64 {
	return c.nodeSize() * int64(index)
}

func (c converter) nodeSize() int64 {
	return int64(4 + // n_descendants
		4*2 + // children[2]
		8*c.dim) // v[1]
}

func (c converter) initializeMaps(path string) (map[int]int, error) {
	maps := map[int]int{}
	file, err := os.Open(path)
	if err != nil {
		return maps, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return maps, err
		}
		key, err := strconv.Atoi(record[0])
		if err != nil {
			return maps, err
		}

		index, err := strconv.Atoi(record[1])
		if err != nil {
			return maps, err
		}
		maps[index] = key
	}

	return maps, nil
}

type csvConverter struct {
	dim   int
	tree  int
	K     int
	order binary.ByteOrder
}

func (c csvConverter) Convert(from, path, to, mapPath string) error {
	file, err := os.Open(from)
	if err != nil {
		return err
	}
	defer file.Close()

	err = CreateMeta(path, to, c.tree, c.dim, c.K)
	if err != nil {
		return err
	}

	gannoy, err := NewGannoyIndex(filepath.Join(path, to+".meta"), Angular{}, RandRandom{})
	if err != nil {
		return err
	}
	reader := csv.NewReader(file)

	keys := []int{}
	vecs := [][]float64{}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		key, err := strconv.Atoi(record[0])
		if err != nil {
			return err
		}

		vec := make([]float64, c.dim)
		for i, f := range record[1:] {
			if feature, err := strconv.ParseFloat(f, 64); err != nil {
				return err
			} else {
				vec[i] = feature
			}
		}
		keys = append(keys, key)
		vecs = append(vecs, vec)
	}
	return gannoy.AddItems(keys, vecs)
}
