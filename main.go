package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	cmd := Cmd{}
	flag.Parse()
	cmd.Filename = flag.Arg(0)
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

type Cmd struct {
	Filename string
}

func (c *Cmd) Run() error {
	data, err := os.ReadFile(c.Filename)
	if err != nil {
		return err
	}
	tData, err := Unmarshal(data)
	if err != nil {
		return err
	}

	// pid -> stack events
	stacks := map[int][]*StackEvent{}
	for _, te := range tData.Events {
		if te.Ph != "B" && te.Ph != "E" {
			continue
		}
		events := stacks[int(te.Pid)]
		event := &StackEvent{}
		if len(events) > 0 {
			event = events[len(events)-1]
		}
		timeNS := int64(te.Ts) * 1000
		if event.Stack == nil || event.TimeNS != timeNS {
			newEvent := &StackEvent{
				TimeNS: timeNS,
				Stack:  make([]string, len(event.Stack)),
			}
			copy(newEvent.Stack, event.Stack)
			event = newEvent
			stacks[int(te.Pid)] = append(events, event)
		}

		switch te.Ph {
		case "B":
			event.Stack = append(event.Stack, te.Name)
		case "E":
			event.Stack = event.Stack[0 : len(event.Stack)-1]
		}
	}

	out := &Iteration2Format{
		Threads: map[string][]*Iteration2Event{},
		Version: "1.2",
	}
	ft := NewFrameTable()
	for goroutineID, events := range stacks {
		threadID := fmt.Sprintf("G%d", goroutineID)
		for i, se := range events {
			if len(se.Stack) == 0 {
				continue
			}
			event := &Iteration2Event{
				StartNS: se.TimeNS,
				Label:   se.Stack[0],
			}
			if i+1 < len(events) {
				event.EndNS = events[i+1].TimeNS
				if event.EndNS > out.TimeRange.EndNS {
					out.TimeRange.EndNS = event.EndNS
				}
			}
			for _, method := range se.Stack[1:] {
				frame := Frame{Method: method}
				idx := ft.Lookup(frame)
				event.Stack = append(event.Stack, idx)
			}
			out.Threads[threadID] = append(out.Threads[threadID], event)
		}
	}
	st := NewStringTable()
	out.Frames = ft.Frames(st)
	out.Strings = st.Strings()
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	return e.Encode(out)
}

func NewStringTable() *StringTable {
	return &StringTable{strings: map[string]int{}}
}

type StringTable struct {
	strings map[string]int
}

func (t *StringTable) Lookup(str string) int {
	if idx, ok := t.strings[str]; ok {
		return idx
	}
	idx := len(t.strings)
	t.strings[str] = idx
	return idx
}

func (t *StringTable) Strings() []string {
	list := make([]string, len(t.strings))
	for str, idx := range t.strings {
		list[idx] = str
	}
	return list
}

func NewFrameTable() *FrameTable {
	return &FrameTable{frames: map[Frame]int{}}
}

type FrameTable struct {
	frames map[Frame]int
}

func (t *FrameTable) Lookup(frame Frame) int {
	if idx, ok := t.frames[frame]; ok {
		return idx
	}
	idx := len(t.frames)
	t.frames[frame] = idx
	return idx
}

func (t *FrameTable) Frames(st *StringTable) [][]int {
	list := make([][]int, len(t.frames))
	for frame, idx := range t.frames {
		list[idx] = []int{
			st.Lookup(frame.Filename),
			st.Lookup(frame.Package),
			st.Lookup(frame.Class),
			st.Lookup(frame.Method),
			frame.Line,
		}
	}

	return list
}

type Frame struct {
	Filename string
	Package  string
	Class    string
	Method   string
	Line     int
}

type StackEvent struct {
	TimeNS int64
	Stack  []string
}

func Unmarshal(data []byte) (*TraceData, error) {
	var tr TraceData
	return &tr, json.Unmarshal(data, &tr.Events)
}

type TraceData struct {
	Events []*TraceEvent
}

type TraceEvent struct {
	Name string `json:"name,omitempty"`
	Ph   string `json:"ph,omitempty"`
	// Ts is the tracing clock timestamp of the event. The timestamps are
	// provided at microsecond granularity.
	Ts   float64                `json:"ts"`
	Pid  int64                  `json:"pid,omitempty"`
	Tid  int64                  `json:"tid,omitempty"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type Iteration2Format struct {
	Threads   map[string][]*Iteration2Event `json:"threads"`
	TimeRange Iteration2TimeRange           `json:"timeRange"`
	Frames    [][]int                       `json:"frames"`
	Strings   []string                      `json:"strings"`
	Version   string                        `json:"version"`
}

type Iteration2Event struct {
	StartNS int64  `json:"startNs"`
	EndNS   int64  `json:"endNs"`
	Label   string `json:"label"`
	Stack   []int  `json:"stack"`
}

type Iteration2TimeRange struct {
	StartNS int64 `json:"startNs"`
	EndNS   int64 `json:"endNs"`
}
