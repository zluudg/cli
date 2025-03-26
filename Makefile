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

netbsd:  
	/bin/sh make-version.sh $(VERSION)-$(COMMIT) $(APPDATE) $(PROG)
	GOOS=netbsd GOARCH=amd64 go build $(GOFLAGS) -o ${PROG}.netbsd

test:
	$(GO) test -v -cover

clean:
	@rm -f $(PROG) *~ cmd/*~

install:
	install -b -c -s ${PROG} /usr/local/bin/

rpm: build
	sed -e "s/@@VERSION@@/$(VERSION)/g" rpm/nfpm.yaml.in > rpm/nfpm.yaml
	nfpm pkg -f rpm/nfpm.yaml --packager rpm --target ./rpm/out


.PHONY: build clean

