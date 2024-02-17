O ?= .

$(O)/if-inject: cmd/if-inject/main.go pkg/util/netns.go pkg/util/util.go
	./build.sh static --dest=$(O)

.PHONY: clean
clean:
	rm -f $(O)/if-inject
