.PHONY: test lint install check

test:
	/bin/bash test/run.sh

lint:
	shellcheck -s bash -x bin/dwf install.sh server/setup-server.sh test/run.sh

check: lint test

install:
	./install.sh
