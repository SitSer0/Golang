//go:build !solution

package externalsort

import (
	"bufio"
	"container/heap"
	"fmt"
	"io"
	"os"
	"sort"
)

func NewReader(r io.Reader) LineReader {
	return &lineReader{
		reader: bufio.NewReader(r),
	}
}

func NewWriter(w io.Writer) LineWriter {
	return &lineWriter{
		writer: bufio.NewWriter(w),
	}
}

type Heap []string

func (h Heap) Len() int           { return len(h) }
func (h Heap) Less(i, j int) bool { return h[i] < h[j] }
func (h Heap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *Heap) Push(x interface{}) {
	*h = append(*h, x.(string))
}

func (h *Heap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func Merge(w LineWriter, readers ...LineReader) error {
	flag := true
	h := &Heap{}
	heap.Init(h)
	for flag {
		flag = false
		for _, reader := range readers {
			line, err := reader.ReadLine()
			if err != nil {
				if err == io.EOF {
					if line != "" {
						heap.Push(h, line)
					}
					continue
				}
				return err
			}
			flag = true
			fmt.Println(line)
			heap.Push(h, line)
		}
	}

	for h.Len() > 0 {
		line := heap.Pop(h).(string)
		err := w.Write(line)
		if err != nil {
			if err == io.EOF {
				continue
			}
			return err
		}
	}
	return nil
}

func Sort(w io.Writer, in ...string) error {
	readers := make([]LineReader, len(in))
	for readerIndex, file := range in {
		currentInputFile, err := os.OpenFile(file, os.O_RDWR, 0644)
		if err != nil {
			return err
		}
		defer currentInputFile.Close()
		readers[readerIndex] = LineReader(NewReader(currentInputFile))
		linesToSort := make([]string, 0)

		var currentLine string
		for err != io.EOF {
			currentLine, err = readers[readerIndex].ReadLine()
			if err != nil && err != io.EOF {
				return err
			} else if err == nil {
				linesToSort = append(linesToSort, currentLine)
			} else if len(currentLine) > 0 && err == io.EOF {
				linesToSort = append(linesToSort, currentLine)
			}
		}
		fmt.Println(linesToSort)
		sort.Strings(linesToSort)
		fmt.Println(linesToSort)
		_, err = currentInputFile.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}
		//err = currentInputFile.Truncate(0)
		if err != nil {
			return err
		}
		writerSorted := LineWriter(NewWriter(currentInputFile))
		for _, line := range linesToSort {
			err = writerSorted.Write(line)
			if err != nil {
				return err
			}
		}
		_, _ = currentInputFile.Seek(0, io.SeekStart)
	}
	fmt.Println("ABOBA")
	writer := NewWriter(w)
	err := Merge(writer, readers...)

	if err != nil {
		return err
	}

	return nil
}
