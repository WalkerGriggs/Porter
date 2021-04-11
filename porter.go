package porter

import (
	"fmt"
	"math/rand"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// DefaultConfig is used to set reasonable config defaults.
func DefaultConfig() *Config {
	return &Config{
		BlockSize:  100,
		MaxBlocks:  10,
		LowerBound: 10000,
	}
}

// Config is used to set Porter port block parameters. Generally, the default
// config values are sufficient.
type Config struct {
	// BlockSize is used to configure the port block size.
	BlockSize int

	// MaxBlocks is used to configure the number of port blocks. The number
	// of blocks may be trimmed depending on the ephemeral port range of
	// the host system.
	MaxBlocks int

	// LowerBound is used to configure the lowest port Porter will allocate.
	LowerBound int
}

// Porter is used to track free ports.
type Porter struct {
	config *Config

	// effectiveMaxBlocks is the number of port blocks adjusted to the
	// ephemeral port range of the host systems.
	effectiveMaxBlocks int

	// firstIP is the first IP of the allocated block
	firstPort int

	// freePorts is the list of ports _we believe_ to be free
	freePorts []int

	// ln is used to reserve the port block on the host
	ln net.Listener

	// mu is used to force synchronous edits on the port lists
	mu sync.Mutex
}

// New creates a new Porter object. It returns an error if porter is unable to
// allocate the port block.
func New(config *Config) (*Porter, error) {
	p := &Porter{
		config:             config,
		freePorts:          make([]int, 0),
		effectiveMaxBlocks: config.MaxBlocks,
	}

	if err := p.adjustMaxBlocks(); err != nil {
		return nil, err
	}

	if p.effectiveMaxBlocks <= 0 {
		return nil, fmt.Errorf("No port blocks available outside of range")
	}

	if config.LowerBound+(p.effectiveMaxBlocks*config.BlockSize) > 65535 {
		return nil, fmt.Errorf("Block size too big or too many blocks allocated")
	}

	// Allocate a port block
	rand.Seed(time.Now().UnixNano())
	p.alloc()

	// Select free ports from the allocated port block
	for port := p.firstPort + 1; port < p.firstPort+config.BlockSize; port++ {
		if used := IsPortInUse(port); !used {
			p.freePorts = append(p.freePorts, port)
		}
	}

	return p, nil
}

// alloc is used to allocate a new port block and take out a listener lock.
func (p *Porter) alloc() error {
	fmt.Println(int32(p.effectiveMaxBlocks))

	// grab a random first port from the effective block range
	block := int(rand.Int31n(int32(p.effectiveMaxBlocks)))
	first := p.config.LowerBound + (block * p.config.BlockSize)

	// lock the port by taking out a listener. This must be freed.
	ln, err := net.ListenTCP("tcp", TCPAddr("127.0.0.1", first))
	if err != nil {
		return err
	}

	p.firstPort, p.ln = first, ln
	return nil
}

// adjustMaxBlocks checks for block overlap with the ephemeral port range. If
// there is overlap, cut the MaxBlocks short.
func (p *Porter) adjustMaxBlocks() error {
	ephemeralMin, ephemeralMax, err := ephemeralPortRange()
	if err != nil {
		return err
	}

	// check to see if any of the blocks overlap with the ephemeral range. If so,
	// lower the maxBlock.
	for block := 0; block < p.config.MaxBlocks; block++ {
		min := p.config.LowerBound + (block * p.config.BlockSize)
		max := min + p.config.BlockSize
		overlap := rangeOverlap(min, max-1, ephemeralMin, ephemeralMax)
		if overlap {
			p.effectiveMaxBlocks = block
		}

	}

	return nil
}

// Take is used to take a list of free ports.
func (p *Porter) Take(n int) ([]int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if n > len(p.freePorts) {
		return nil, fmt.Errorf("Block size too small")
	}
	ports := make([]int, 0)

	for len(ports) < n {
		port := p.freePorts[0]
		p.freePorts = p.freePorts[1:]

		if used := IsPortInUse(port); used {
			continue
		}

		ports = append(ports, port)
	}

	return ports, nil
}

// MustTake is used to take a list of free ports, and panics if there's an
// error.
func (p *Porter) MustTake(n int) (ports []int) {
	ports, err := p.Take(n)
	if err != nil {
		panic(err)
	}
	return ports
}

// Close is used to close the listener that locks the first port of the
// allocated block.
func (p *Porter) Close() {
	if p.ln != nil {
		p.ln.Close()
		p.ln = nil
	}
}

// TCPAddr is used to initialize a net.TCPAddr from a given ip/port string/int.
func TCPAddr(ip string, port int) *net.TCPAddr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: port}
}

// ephemeralPortRange is used to get the host systems's ephemeral port range.
// This function is a bit of hack, and needs to be expanded to support Darwin
// and Windows.
func ephemeralPortRange() (int, int, error) {
	key := "net.ipv4.ip_local_port_range"
	pattern := regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s*$`)

	cmd := exec.Command("sysctl", "-n", key)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}

	val := string(out)

	m := pattern.FindStringSubmatch(val)
	if m != nil {
		min, err1 := strconv.Atoi(m[1])
		max, err2 := strconv.Atoi(m[2])

		if err1 == nil && err2 == nil {
			return min, max, nil
		}
	}

	return 0, 0, fmt.Errorf("Unexpected sysctl value %q.", val)
}

// rangeOverlap is a predicate used to check if the two min-max pairs overload.
func rangeOverlap(min1, max1, min2, max2 int) bool {
	if min1 > max1 {
		return false
	}
	if min2 > max2 {
		return false
	}
	return min1 <= max2 && min2 <= max1
}

// IsPortInUse is a predicate used to check if a process is already bound to
// given port.
func IsPortInUse(port int) bool {
	ln, err := net.ListenTCP("tcp", TCPAddr("127.0.0.1", port))
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}
