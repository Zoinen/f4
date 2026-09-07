package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

type RecordEntry struct {
	Time float64        `json:"time"`
	Dir  string         `json:"dir"`
	Msg  map[string]any `json:"msg"`
}

type CastHeader struct {
	Version   int            `json:"version"`
	Width     int            `json:"width"`
	Height    int            `json:"height"`
	Timestamp int64          `json:"timestamp,omitempty"`
	Title     string         `json:"title,omitempty"`
	Env       map[string]any `json:"env,omitempty"`
}

func main() {
	outFlag := flag.String("o", "", "Output asciicast file path")
	titleFlag := flag.String("title", "vtui session", "Asciicast title")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: vtui-cast [options] <session.jsonl>")
		os.Exit(1)
	}

	srcPath := args[0]
	f, err := os.Open(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vtui-cast: cannot open %s: %v\n", srcPath, err)
		os.Exit(1)
	}
	defer f.Close()

	outPath := *outFlag
	if outPath == "" {
		outPath = "session.cast"
	}
	outF, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vtui-cast: cannot create %s: %v\n", outPath, err)
		os.Exit(1)
	}
	defer outF.Close()

	if err := convertToCast(f, outF, *titleFlag); err != nil {
		fmt.Fprintf(os.Stderr, "vtui-cast: conversion failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Exported asciicast to %s\n", outPath)
}

func convertToCast(r io.Reader, w io.Writer, title string) error {
	header := CastHeader{
		Version: 2,
		Width:   80,
		Height:  25,
		Title:   title,
	}
	hBytes, _ := json.Marshal(header)
	if _, err := w.Write(append(hBytes, '\n')); err != nil {
		return err
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry RecordEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Dir == "up" {
			data, _ := json.Marshal(entry.Msg)
			castEvent := []any{entry.Time, "o", string(data) + "\r\n"}
			evBytes, _ := json.Marshal(castEvent)
			if _, err := w.Write(append(evBytes, '\n')); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
