VER=1.0~0.$(shell date +%s)
DEBARCH=amd64

all: unlockr

unlockr: unlockr.src
	go build -o $@ .

unlockr.linux: unlockr.src
	GOARCH=$(DEBARCH) GOOS=linux go build -o $@ .

unlockr.src: *.go *.html */*.go */*.js

unlockr.deb: unlockr.linux systemd/* util/build-deb/build-deb util/build-deb/spec.yaml util/build-deb/postinst
	util/build-deb/build-deb \
		-c util/build-deb/spec.yaml \
		-o $@ \
		-version $(VER) \
		-postinst util/build-deb/postinst \
		unlockr.linux:/usr/bin/unlockr \
		config-sample.json:/etc/unlockr/config-sample.json \
		users-sample.json:/etc/unlockr/users-sample.json \
		systemd/unlockr.service:/usr/lib/systemd/system/unlockr.service

	tar -xOf unlockr.deb control.tar.gz | tar -tv
	tar -xOf unlockr.deb data.tar.gz | tar -tv

util/build-deb/build-deb: util/build-deb/*.go
	go build -o $@ ./util/build-deb

clean:
	rm -fv \
		unlockr \
		unlockr.linux \
		unlockr.deb \
		util/hash/hash \
		util/build-deb/build-deb

.PHONY: all clean unlockr.src
