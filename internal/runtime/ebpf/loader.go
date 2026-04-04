package ebpf

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/google/uuid"
)

// EBPFEvent represents a generic event from eBPF
type EBPFEvent struct {
	Timestamp uint64
	PID       uint32
	PPID      uint32
	Comm      [16]byte
	Data      []byte // Raw payload from eBPF
}

// Program represents a loaded eBPF program
type Program struct {
	ID           uuid.UUID
	Name         string
	Collection   *ebpf.Collection
	Links        []link.Link
	RingBuf      *ringbuf.Reader
	ctx          context.Context
	cancel       context.CancelFunc
	eventHandler func(EBPFEvent)
	mu           sync.RWMutex
}

// Loader manages eBPF programs
type Loader struct {
	programs map[uuid.UUID]*Program
	mu       sync.RWMutex
}

// NewLoader creates a new eBPF loader
func NewLoader() (*Loader, error) {
	logger.Info("Creating eBPF loader...")

	// Remove resource limits for eBPF (may fail in Docker without privileged mode)
	if err := rlimit.RemoveMemlock(); err != nil {
		// This is expected in Docker containers without CAP_SYS_RESOURCE
		// eBPF will still work for small programs
		logger.Debug("Memlock limit not removed (expected in Docker)", "error", err.Error())
	}

	logger.Info("✅ eBPF loader created")

	return &Loader{
		programs: make(map[uuid.UUID]*Program),
	}, nil
}

// LoadProgram loads an eBPF program from bytes
func (l *Loader) LoadProgram(ctx context.Context, pluginID uuid.UUID, name string, programBytes []byte, eventHandler func(EBPFEvent)) (*Program, error) {
	return l.LoadProgramWithManifest(ctx, pluginID, name, programBytes, nil, eventHandler)
}

// LoadProgramWithManifest loads an eBPF program with manifest for correct tracepoint mapping
func (l *Loader) LoadProgramWithManifest(ctx context.Context, pluginID uuid.UUID, name string, programBytes []byte, ebpfPrograms []struct{ Name, Attach string }, eventHandler func(EBPFEvent)) (*Program, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	logger.Info("Loading eBPF program", "plugin_id", pluginID.String(), "name", name, "size", len(programBytes))

	// Load collection spec from bytes
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(programBytes))
	if err != nil {
		logger.Error("Failed to load eBPF collection spec",
			"plugin_id", pluginID.String(),
			"error", err.Error())
		return nil, fmt.Errorf("failed to load collection spec: %w", err)
	}

	// Load collection
	collection, err := ebpf.NewCollection(spec)
	if err != nil {
		logger.Error("Failed to load eBPF collection",
			"plugin_id", pluginID.String(),
			"error", err.Error())
		return nil, fmt.Errorf("failed to load collection: %w", err)
	}

	// Find ringbuf map
	var ringBufMap *ebpf.Map
	for mapName, m := range collection.Maps {
		if m.Type() == ebpf.RingBuf {
			ringBufMap = m
			logger.Debug("Found ringbuf map", "name", mapName, "plugin_id", pluginID.String())
			break
		}
	}

	// Create ringbuf reader
	var ringBufReader *ringbuf.Reader
	if ringBufMap != nil {
		var err error
		ringBufReader, err = ringbuf.NewReader(ringBufMap)
		if err != nil {
			logger.Error("Failed to create ringbuf reader",
				"plugin_id", pluginID.String(),
				"error", err.Error())
			collection.Close()
			return nil, fmt.Errorf("failed to create ringbuf reader: %w", err)
		}
	}

	// Attach programs based on their type
	links := make([]link.Link, 0)
	for progName, prog := range collection.Programs {
		logger.Info("Attaching eBPF program", "name", progName, "type", prog.Type(), "plugin_id", pluginID.String())

		// Try to attach based on manifest info first
		attached, err := l.attachProgram(prog, progName, ebpfPrograms)
		if err != nil {
			logger.Warn("Failed to attach program",
				"plugin_id", pluginID.String(),
				"name", progName,
				"error", err.Error())
			// Continue anyway - program might be for manual attachment
		} else if attached != nil {
			links = append(links, attached)
			logger.Info("Attached eBPF program",
				"plugin_id", pluginID.String(),
				"name", progName)
		} else {
			logger.Warn("eBPF program not attached (no matching tracepoint/kprobe)",
				"plugin_id", pluginID.String(),
				"name", progName,
				"type", prog.Type())
		}
	}

	// Create context
	ctx, cancel := context.WithCancel(ctx)

	// Create program
	program := &Program{
		ID:           pluginID,
		Name:         name,
		Collection:   collection,
		Links:        links,
		RingBuf:      ringBufReader,
		ctx:          ctx,
		cancel:       cancel,
		eventHandler: eventHandler,
	}

	l.programs[pluginID] = program

	// Start reading events
	if ringBufReader != nil {
		go program.readEvents()
	}

	logger.Info("✅ eBPF program loaded",
		"plugin_id", pluginID.String(),
		"programs", len(collection.Programs),
		"maps", len(collection.Maps),
		"links", len(links))

	return program, nil
}

// attachProgram attempts to attach an eBPF program based on manifest or program type
func (l *Loader) attachProgram(prog *ebpf.Program, progName string, ebpfPrograms []struct{ Name, Attach string }) (link.Link, error) {
	progType := prog.Type()

	logger.Debug("Program info",
		"name", progName,
		"type", progType)

	// Try to attach based on manifest first
	if ebpfPrograms != nil {
		for _, ep := range ebpfPrograms {
			if ep.Name == progName && ep.Attach != "" {
				// Check if it's a raw_tracepoint (no "/" in attach string)
				if !strings.Contains(ep.Attach, "/") {
					// Raw tracepoint - attach without group
					lk, err := link.AttachRawTracepoint(link.RawTracepointOptions{
						Name:    ep.Attach,
						Program: prog,
					})
					if err == nil {
						logger.Info("Attached raw tracepoint from manifest", "name", ep.Attach)
						return lk, nil
					}
					logger.Warn("Failed to attach raw tracepoint from manifest",
						"name", ep.Attach,
						"error", err.Error())
				} else {
					// Regular tracepoint: parse attach string "group/name" → group=group, name=name
					parts := strings.SplitN(ep.Attach, "/", 2)
					if len(parts) == 2 {
						lk, err := link.Tracepoint(parts[0], parts[1], prog, nil)
						if err == nil {
							logger.Info("Attached tracepoint from manifest", "group", parts[0], "name", parts[1])
							return lk, nil
						}
						logger.Warn("Failed to attach tracepoint from manifest",
							"group", parts[0],
							"name", parts[1],
							"error", err.Error())
					}
				}
			}
		}
	}

	// Try different attachment methods based on program type
	switch progType {
	case ebpf.TracePoint:
		// Fallback: try common tracepoints (manifest should have provided correct one)
		fallbackTracepoints := []struct{ group, name string }{
			{"syscalls", "sys_enter_openat"},
			{"syscalls", "sys_enter_open"},
			{"sched", "sched_process_fork"},
			{"sched", "sched_process_exit"},
		}

		for _, tp := range fallbackTracepoints {
			lk, err := link.Tracepoint(tp.group, tp.name, prog, nil)
			if err == nil {
				logger.Info("Attached fallback tracepoint", "group", tp.group, "name", tp.name)
				return lk, nil
			}
		}

		return nil, fmt.Errorf("no suitable tracepoint found for program %s", progName)

	case ebpf.Kprobe:
		// Try common kprobes
		kprobes := []string{
			"tcp_connect",
			"tcp_sendmsg",
			"tcp_recvmsg",
			"do_sys_open",
			"do_sys_openat2",
			"security_file_open",
		}

		for _, kp := range kprobes {
			lk, err := link.Kprobe(kp, prog, nil)
			if err == nil {
				return lk, nil
			}
		}
		return nil, nil

	case ebpf.XDP:
		// XDP programs need to be attached to a specific interface
		// This is typically done externally
		return nil, fmt.Errorf("XDP program requires manual attachment to interface")

	case ebpf.SocketFilter:
		// Socket filter programs are attached to sockets
		return nil, fmt.Errorf("socket filter program requires manual attachment")

	case ebpf.SchedCLS:
		// TC (traffic control) programs
		return nil, fmt.Errorf("TC program requires manual attachment to interface")

	case ebpf.CGroupSKB:
		// CGroup programs
		return nil, fmt.Errorf("cgroup program requires manual attachment to cgroup")

	default:
		logger.Debug("Unknown program type, skipping attachment",
			"name", progName,
			"type", progType)
		return nil, nil
	}
}

// readEvents reads events from ringbuf
func (p *Program) readEvents() {
	logger.Debug("Starting ringbuf event reader", "plugin_id", p.ID.String())

	for {
		select {
		case <-p.ctx.Done():
			logger.Debug("Ringbuf event reader stopped", "plugin_id", p.ID.String())
			return

		default:
			if p.RingBuf == nil {
				continue
			}

			record, err := p.RingBuf.Read()
			if err != nil {
				logger.Error("Ringbuf read error",
					"plugin_id", p.ID.String(),
					"error", err.Error())
				continue
			}

			// Parse event - extract common header (timestamp + pid + ppid + comm)
			if len(record.RawSample) >= 32 { // Minimum: 8 + 4 + 4 + 16
				event := parseEBPFEvent(record.RawSample)

				p.mu.RLock()
				if p.eventHandler != nil {
					p.eventHandler(event)
				}
				p.mu.RUnlock()
			}
		}
	}
}

// parseEBPFEvent parses raw bytes from eBPF ringbuf
// Expected eBPF structure:
//   __u64 timestamp;    // 0-8
//   __u32 pid;          // 8-12
//   char comm[16];      // 12-28
//   char type;          // 28
func parseEBPFEvent(data []byte) EBPFEvent {
	var event EBPFEvent

	if len(data) < 29 {
		logger.Debug("eBPF event too small", "size", len(data))
		return event
	}

	event.Timestamp = binary.LittleEndian.Uint64(data[0:8])
	event.PID = binary.LittleEndian.Uint32(data[8:12])
	copy(event.Comm[:], data[12:28])

	// type is at offset 28
	eventType := data[28]
	event.Data = []byte{eventType}

	logger.Debug("Parsed eBPF event",
		"timestamp", event.Timestamp,
		"pid", event.PID,
		"comm", string(event.Comm[:]),
		"type", eventType,
		"raw_size", len(data))

	return event
}

// GetProgram returns a program by ID
func (l *Loader) GetProgram(programID uuid.UUID) (*Program, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	prog, ok := l.programs[programID]
	if !ok {
		return nil, fmt.Errorf("program not found: %s", programID.String())
	}

	return prog, nil
}

// UnloadProgram unloads an eBPF program
func (l *Loader) UnloadProgram(programID uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	prog, ok := l.programs[programID]
	if !ok {
		return fmt.Errorf("program not found: %s", programID.String())
	}

	logger.Info("Unloading eBPF program", "plugin_id", programID.String())

	// Cancel context
	prog.cancel()

	// Close ringbuf
	if prog.RingBuf != nil {
		prog.RingBuf.Close()
	}

	// Close links
	for _, lk := range prog.Links {
		lk.Close()
	}

	// Close collection
	prog.Collection.Close()

	// Remove from programs
	delete(l.programs, programID)

	logger.Info("✅ eBPF program unloaded", "plugin_id", programID.String())

	return nil
}

// Close closes the loader
func (l *Loader) Close() error {
	logger.Info("Closing eBPF loader...")

	l.mu.Lock()
	defer l.mu.Unlock()

	// Unload all programs
	for id := range l.programs {
		if err := l.UnloadProgram(id); err != nil {
			logger.Error("Failed to unload program during close",
				"plugin_id", id.String(),
				"error", err.Error())
		}
	}

	logger.Info("✅ eBPF loader closed")
	return nil
}
