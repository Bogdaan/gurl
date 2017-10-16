CMD=go build -v -o gurl
GOOS=export GOOS
GOARCH=export GOARCH
LINUX=$(GOOS)=linux
OSX=$(GOOS)=darwin

windows64:
	$(GOOS)=windows && $(GOARCH)=amd64 && go build -v -o gurl.exe \
	&& zip windows64.zip gurl.exe && rm gurl.exe

linux_arm:
	$(LINUX) && $(GOARCH)=arm && $(CMD) \
		&& tar czf linux_arm.tar.gz gurl && rm gurl

linux64:
	$(LINUX) && $(GOARCH)=amd64 && $(CMD) \
		&& tar czf linux64.tar.gz gurl && rm gurl

linux386:
	$(LINUX) && $(GOARCH)=386 && $(CMD) \
		&& tar czf linux386.tar.gz gurl && rm gurl

osx64:
	$(OSX) && $(GOARCH)=amd64 && $(CMD) \
		&& tar czf osx64.tar.gz gurl && rm gurl

osx386:
	$(OSX) && $(GOARCH)=386 && $(CMD) \
		&& tar czf osx386.tar.gz gurl && rm gurl

all: windows64 linux64 linux386 linux_arm osx64 osx386
