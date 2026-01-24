# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Okura-Node is a quantum-resistant blockchain node written in Go 1.23.6. It features post-quantum cryptography (Falcon-512 and MAYO-5), proof-of-synergy consensus, EVM smart contracts, staking, DEX, and voting systems.

## Build & Run Commands

```bash
# Install dependencies
go get ./...

# Generate a new wallet
go run cmd/generateNewWallet/main.go

# Run mining node (requires peer IP)
go run cmd/mining/main.go <peer_ip>

# Run GUI wallet
go run cmd/gui/main.go

# Run tests
go test ./...
go test ./account         # single package
go test -v ./wallet       # verbose output
```

## Required System Dependencies

- RocksDB v10.2.1 (librocksdb-dev)
- liboqs (commit 8ee6039) for post-quantum cryptography
- Qt5 for GUI (qtbase5-dev)
- ZMQ (libzmq3-dev)

## Architecture

### Core Packages

| Package | Purpose |
|---------|---------|
| `cmd/mining/` | Mining node entry point |
| `cmd/gui/` | Qt-based wallet GUI |
| `account/` | Account, staking, and DEX state management |
| `blocks/` | Block creation and processing |
| `core/evm/` | Ethereum Virtual Machine implementation |
| `core/stateDB/` | Contract state persistence |
| `crypto/oqs/` | Post-quantum crypto bindings (Falcon-512, MAYO-5) |
| `database/` | RocksDB abstraction layer |
| `services/transactionServices/` | Transaction validation and P2P distribution |
| `services/syncService/` | Blockchain synchronization with peers |
| `tcpip/` | Custom P2P networking |
| `transactionsPool/` | Memory pool with Merkle tree verification |
| `wallet/` | Wallet creation, encryption, and key management |

### Key Data Types (in `common/`)

- **Address**: 20 bytes, account identifier
- **Hash**: 32 bytes (Blake2b), used for txs/blocks/merkle
- **Signature**: Variable (Falcon-512: 752 bytes, MAYO-5: 964 bytes)

### Database Prefix System

RocksDB uses 2-byte prefixes: `BI` (blocks), `TT` (transactions), `AC` (accounts), `SA` (staking), `DA` (DEX), `PK` (public keys), `HB` (headers), `BH` (blocks by height).

### Dual Signature System

Transactions use two post-quantum signature schemes:
1. Primary: Falcon-512 (pub: 897B, sig: 752B)
2. Secondary: MAYO-5 (pub: 5554B, sig: 964B)

### Concurrency Patterns

- RWMutex protects account/state maps
- Package-level singletons: `account.Accounts`, `common.IsSyncing`
- Services run as background goroutines
- P2P messages routed via channels

### Network Ports

Open these TCP ports: 19023 (transactions), 18023 (nonce), 17023 (self-nonce), 16023 (sync)

## Configuration

Runtime config in `~/.okura/.env`:
```
DELEGATED_ACCOUNT=1          # Staking account (1-254)
REWARD_PERCENTAGE=200        # Operator reward (0-500, where 500=50%)
NODE_IP=<your_external_ip>
HEIGHT_OF_NETWORK=<current_height>
```

Genesis config: `~/.okura/genesis/config/genesis.json` (copy from `genesis/config/genesis_internal_tests.json`)

## Network Constants

- Block interval: 10 seconds
- Max transactions per block: 5000
- Max peers: 6
- Decimals: 8
- Minimum staking for node: 1,000,000 (with 8 decimals)

## Commit Convention

Use task identifiers in commits (e.g., `OB-55 description`).
