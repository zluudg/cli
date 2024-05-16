PROG:=tapir-cli
# -----
VERSION:=`cat ./VERSION`
COMMIT:=`git describe --dirty=+WiP --always`
APPDATE=`date +"%Y-%m-%d-%H:%M"`
GOFLAGS:=-v -ldflags "-X app.version=$(VERSION)-$(COMMIT)"

GOOS ?= $(shell uname -s | tr A-Z a-z)

GO:=GOOS=$(GOOS) CGO_ENABLED=0 go
# GO:=GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=1 go

default: ${PROG}

${PROG}: build

build:  
	/bin/sh make-version.sh $(VERSION)-$(COMMIT) $(APPDATE) $(PROG)
	$(GO) build $(GOFLAGS) -o ${PROG}

linux:  
	/bin/sh make-version.sh $(VERSION)-$(COMMIT) $(APPDATE) $(PROG)
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o ${PROG}.linux
# ----
# VERSION:=$(shell git describe --dirty=+WiP --always)
# VERSION:=`git describe --dirty=+WiP --always`
# APPDATE=`date +"%Y-%m-%d-%H:%M"`

# GOFLAGS:=-v -ldflags "-X app.version=$(VERSION) -v"

# GOOS ?= $(shell uname -s | tr A-Z a-z)
# GOARCH:=amd64

# GO:=GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go

# default: ${PROG}

# ${PROG}: build

# build:
# 	/bin/sh make-version.sh $(VERSION) ${APPDATE}
# 	$(GO) build $(GOFLAGS) -o ${PROG}

test:
	$(GO) test -v -cover

clean:
	@rm -f $(PROG) *~ cmd/*~

install:
	install -b -c -s ${PROG} /usr/local/bin/

.PHONY: build clean

