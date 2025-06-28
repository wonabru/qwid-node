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
    sudo apt install nano git
    git config --global credential.helper store

Install OQS library:

    git clone https://github.com/open-quantum-safe/liboqs.git
    cd liboqs/
    git checkout 8ee6039 
    
Compile OQS with `-DBUILD_SHARED_LIBS=ON` and install
    
    mkdir build && cd build
    cmake -GNinja -DBUILD_SHARED_LIBS=ON ..    
    ninja
    sudo ninja install
    cd ~/

Install go1.23.6 if not installed:

    wget https://go.dev/dl/go1.23.6.linux-amd64.tar.gz
    sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.23.6.linux-amd64.tar.gz

Add on the end of ~/.bashrc

    export PATH=$PATH:/usr/local/go/bin

reload shell:

    bash

check instalation of go

    go version

Follow instruction from https://github.com/open-quantum-safe/liboqs-go.git in order to install go wrapper to oqs. Finally

    git clone --depth=1 https://github.com/open-quantum-safe/liboqs-go

Edit: liboqs-go/.config/liboqs-go.pc

and should be like this:

    LIBOQS_INCLUDE_DIR=/usr/local/include
    LIBOQS_LIB_DIR=/usr/local/lib
    
    Name: liboqs-go
    Description: Go bindings for liboqs, a C library for quantum resistant cryptography
    Version: 0.13.0-dev
    Cflags: -I${LIBOQS_INCLUDE_DIR}
    Ldflags: '-extldflags "-Wl,-stack_size -Wl,0x1000000"'
    Libs: -L${LIBOQS_LIB_DIR} -loqs

On the end of ~/.bashrc write this line:

    export PKG_CONFIG_PATH=$PKG_CONFIG_PATH:$HOME/liboqs-go/.config
    

Reload shell and dynamic libraries

    bash
    go clean -cache
    sudo ldconfig -v | grep oqs

Clone project source code

    git clone https://github.com/okuralabs/okura-node.git
    cd okura-node

install go modules

    go get ./...

    mkdir -p ~/.okura/genesis/config

Copy genesis config file ex.:

    cp genesis/config/genesis_internal_tests.json ~/.okura/genesis/config/genesis.json

Copy env file and change accordingly.

    cp .okura/.env ~/.okura/.env

Edit ~/.okura/.env

    DELEGATED_ACCOUNT= any larger rather than 5 but less than 255
    REWARD_PERCENTAGE= any value 0 <= x <= 500    500 ==> means 50% reward to operator
    NODE_IP= your external IP
    WHITELIST_IP= one IP which you want to be be banned
    HEIGHT_OF_NETWORK= current height of network, to speed up syncing. Can be any > 1 but less than blockchain number of mined blocks


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
