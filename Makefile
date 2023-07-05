TMP = /tmp/unlockr.build-deb

all: unlockr

unlockr: *.go *.html */*.go */*.js
	go build .

unlockr.deb: unlockr DEBIAN/* systemd/*
	rm -rfv $(TMP)
	install -d $(TMP)
	install -Dt \
		$(TMP)/DEBIAN/ \
		DEBIAN/control
	install -m 0600 -Dt \
		$(TMP)/etc/unlockr/ \
		config-sample.json \
		users-sample.json
	install -m 0755 -Dt \
		$(TMP)/usr/bin/ \
		unlockr
	install -m 0644 -Dt \
		$(TMP)/usr/lib/systemd/system/ \
		systemd/unlockr.service
	fakeroot dpkg-deb -b $(TMP) $@
	rm -rfv $(TMP)

clean:
	rm -fv \
		unlockr \
		unlockr.deb \
		util/hash/hash

.PHONY: all clean
