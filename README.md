# Porter
A simple package for allocating free ports adapted from [Consul's freeport](https://github.com/hashicorp/consul/tree/master/sdk/freeport).

## Installation

Like most Go packages, installing Porter is easy. First `go get` the package.

```
go get github.com/walkergriggs/porter
```

Next, import Porter.

```
import "github.com/walkergriggs/porter"
```

## Usage

```golang
config := porter.DefaultConfig()

p, err := porter.New(config)
if err != nil {
	panic(err)
}
defer p.Close()

ports := p.MustTake(5)

fmt.Println(ports)

// [10101 10102 10103 10104 10105]
```

## Allocation

Porter takes three configuration variables: `BlockSize`, `MaxBlocks`, and `LowerBound`.

* `BlockSize` configures how many ports are alloted to a block.
* `MaxBlocks` configures how many blocks are considered.
* `LowerBound` configures the lowest allocatable port.

Starting from the lower bound, Porter checks the range of each block, picks one at random, and takes out a lock on the first port. From that block, Porter filters a list of all available ports, which can reserved with `Take` and `MustTake`. 

```
LowerBound = 8000
BlockSize = 100
MaxBlocks = 3

+-----------+-----------+-----------+
| Port Blocks (3)                   |
+-----------+-----------+-----------+
| 8000-8099 | 8100-8199 | 8200-8299 |
+-----------+-----------+-----------+
```

### Ephemeral Ports

Before picking a block and locking the first port, we first adjust the `MaxBlocks` based on the host's ephemeral port range.

If any block overlaps with ephemeral ports, we trim `MaxBlocks` so the allocated blocks end just before the ephemeral ports.

```
LowerBound = 32600
BlockSize = 100
MaxBlocks = 3

+-----------------+-------------+-------------+
| Port Blocks (1) | Free        | Ephemeral  |
+-----------------+-------------+-------------+
| 32600-32699     | 32700-32767 | 32768-60999 |
+-----------------+-------------+-------------+
```
