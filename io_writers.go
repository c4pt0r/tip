package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
)

type ResultIOWriter interface {
	Write(rows []RowResult) error
	Flush() error
}

type CSVResultIOWriter struct {
	writer *csv.Writer
}

func NewCSVResultIOWriter(writer io.Writer) *CSVResultIOWriter {
	return &CSVResultIOWriter{
		writer: csv.NewWriter(writer),
	}
}

func (w *CSVResultIOWriter) Write(rows []RowResult) error {
	for _, row := range rows {
		record := make([]string, len(row.colValues))
		for i, val := range row.colValues {
			record[i] = formatCSVValue(val)
		}
		if err := w.writer.Write(record); err != nil {
			return err
		}
	}
	return nil
}

func (w *CSVResultIOWriter) Flush() error {
	w.writer.Flush()
	return w.writer.Error()
}

type PlainResultIOWriter struct {
	writer *bufio.Writer
}

func NewPlainResultIOWriter(writer io.Writer) *PlainResultIOWriter {
	return &PlainResultIOWriter{
		writer: bufio.NewWriter(writer),
	}
}

func (w *PlainResultIOWriter) Write(rows []RowResult) error {
	for _, row := range rows {
		for i, col := range row.colNames {
			val := row.colValues[i]
			_, err := fmt.Fprintf(w.writer, "%s: %s ", col, formatValue(val))
			if err != nil {
				return err
			}
		}
		_, err := w.writer.WriteString("\n")
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *PlainResultIOWriter) Flush() error {
	return w.writer.Flush()
}

type JSONResultIOWriter struct {
	writer *bufio.Writer
	first  bool
}

func NewJSONResultIOWriter(writer io.Writer) *JSONResultIOWriter {
	return &JSONResultIOWriter{
		writer: bufio.NewWriter(writer),
		first:  true,
	}
}

func (w *JSONResultIOWriter) Write(rows []RowResult) error {
	for _, row := range rows {
		if w.first {
			_, err := w.writer.WriteString("[")
			if err != nil {
				return err
			}
			w.first = false
		} else {
			_, err := w.writer.WriteString(",")
			if err != nil {
				return err
			}
		}

		jsonData, err := json.Marshal(row)
		if err != nil {
			return err
		}

		_, err = w.writer.Write(jsonData)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *JSONResultIOWriter) Flush() error {
	if !w.first {
		_, err := w.writer.WriteString("]")
		if err != nil {
			return err
		}
	}
	return w.writer.Flush()
}
