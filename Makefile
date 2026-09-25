# Build the tablet APK and the server. `make deploy` sends the server to dune.
# `make deploy-apk` installs the APK on the Nexus 7 and starts it again.
# The adb serial is the single line in .tablet, which is not committed.
# The binary is named
# andscreen so the installed unit reads /etc/sysconfig/andscreen.
# servicego names the unit after the executable.
.PHONY: all android server build copy deploy deploy-apk

HOST := dune
BIN  := andscreen
ROOT := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
TABLET := $(strip $(shell cat $(ROOT)/.tablet 2>/dev/null))

all: android server

android:
	$(ROOT)/android/build.sh

deploy-apk: android
	@test -n "$(TABLET)" || { echo "write the adb serial on one line in $(ROOT)/.tablet" >&2; exit 1; }
	adb -s $(TABLET) install -r $(ROOT)/android/build/andscreen.apk
	adb -s $(TABLET) shell am start -n com.andscreen/.MainActivity

server: build

build:
	cd $(ROOT)/server && go build -o /tmp/$(BIN) ./cmd/andscreen

copy: build
	scp $(ROOT)/server/.env $(HOST):/etc/sysconfig/$(BIN)
	scp /tmp/$(BIN) $(HOST):/tmp/$(BIN)

deploy: copy
	ssh $(HOST) /tmp/$(BIN) -action deploy
