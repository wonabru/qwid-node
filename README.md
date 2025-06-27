# Node go OKURA

Works for Ubuntu 22.04 (gcc 11) and go1.23.6
Only one network interface should be with external public IP

Install prerequisites

    sudo apt update
    sudo apt install librocksdb-dev
    sudo apt install libpulse-dev
    sudo apt install libzmq3-dev
    sudo apt install pkg-config
    sudo apt install build-essential
    sudo apt install qtbase5-dev qtchooser qt5-qmake qtbase5-dev-tools
    sudo apt install astyle cmake gcc ninja-build libssl-dev python3-pytest python3-pytest-xdist unzip xsltproc doxygen graphviz python3-yaml valgrind
    sudo apt install nano

Install OQS library:

    git clone https://github.com/open-quantum-safe/liboqs.git
    cd liboqs/
    git checkout 8ee6039 
    
Compile OQS with `-DBUILD_SHARED_LIBS=ON` and install
    
    mkdir build && cd build
    cmake -GNinja -DBUILD_SHARED_LIBS=ON ..    
    ninja
    sudo ninja install

Follow instruction from https://github.com/open-quantum-safe/liboqs-go.git in order to install go wrapper to oqs. Finally

    go clean -cache

Reload dynamic libraries

    sudo ldconfig -v

Clone project source code

    git clone https://github.com/okuralabs/okura-node.git
    cd okura-node
    git config credential.helper store

install go modules

    go get ./...

    mkdir -p ~/.okura/genesis/config

Copy genesis config file ex.:

    cp genesis/config/genesis_testnet.json ~/.okura/genesis/config/genesis.json

Copy env file and change accordingly.

    cp .okura/.env ~/.okura/.env

In the case you are the first who run blockchain and generate genesis block you need to set in .env: DELEGATED_ACCOUNT=1. In other case if you join to other node which is running you can choose unique DELEGATED_ACCOUNT > 1 and < 255.

Ports TCP needed to be opened:

    TransactionTopic: 19023,
    NonceTopic:       18023,
    SelfNonceTopic:   17023,
    SyncTopic:        16023,

localhost port that should be closed from anywhere:

    19009 - wallet - node communication

    8000 - qt requirements (localhost only)


To create account and manage wallet:

    go run cmd/generateNewWallet/main.go

Run Node:

    go run cmd/mining/main.go 46.205.244.17
 
Run GUI:

    go run cmd/gui/main.go
