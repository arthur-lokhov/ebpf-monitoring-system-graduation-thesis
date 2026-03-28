package ebpf

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/google/uuid"
)

// ContainerEvent represents an event from eBPF
type ContainerEvent struct {
	Timestamp uint64
	PID       uint32
	PPID      uint32
	Comm      [16]byte
	Filename  [256]byte
	Type      uint8
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
	eventHandler func(ContainerEvent)
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
	
	// Remove resource limits for eBPF
	if err := rlimit.RemoveMemlock(); err != nil {
		logger.Warn("Failed to remove memlock limit", "error", err.Error())
	}
	
	logger.Info("✅ eBPF loader created")
	
	return &Loader{
		programs: make(map[uuid.UUID]*Program),
	}, nil
}

// LoadProgram loads an eBPF program from bytes
func (l *Loader) LoadProgram(ctx context.Context, pluginID uuid.UUID, name string, programBytes []byte, eventHandler func(ContainerEvent)) (*Program, error) {
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
	for name, m := range collection.Maps {
		if m.Type() == ebpf.RingBuf {
			ringBufMap = m
			logger.Debug("Found ringbuf map", "name", name, "plugin_id", pluginID.String())
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
	
	// Attach programs
	var links []link.Link
	for progName, prog := range collection.Programs {
		logger.Debug("Loading eBPF program", "name", progName, "type", prog.Type())
		
		// TODO: Attach based on program type
		// For now, just store the link
		_ = prog
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
		"maps", len(collection.Maps))
	
	return program, nil
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
			
			// Parse event
			if len(record.RawSample) >= 320 { // Minimum size for ContainerEvent
				event := parseContainerEvent(record.RawSample)
				
				p.mu.RLock()
				if p.eventHandler != nil {
					p.eventHandler(event)
				}
				p.mu.RUnlock()
			}
		}
	}
}

// parseContainerEvent parses raw bytes into ContainerEvent
func parseContainerEvent(data []byte) ContainerEvent {
	var event ContainerEvent
	
	event.Timestamp = binary.LittleEndian.Uint64(data[0:8])
	event.PID = binary.LittleEndian.Uint32(data[8:12])
	event.PPID = binary.LittleEndian.Uint32(data[12:16])
	copy(event.Comm[:], data[16:32])
	copy(event.Filename[:], data[32:288])
	event.Type = data[288]
	
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
