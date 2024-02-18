#! /bin/sh
##
## build.sh --
##
##
## Commands;
##

prg=$(basename $0)
dir=$(dirname $0); dir=$(readlink -f $dir)
tmp=/tmp/${prg}_$$

die() {
    echo "ERROR: $*" >&2
    rm -rf $tmp
    exit 1
}
help() {
    grep '^##' $0 | cut -c3-
    rm -rf $tmp
    exit 0
}
test -n "$1" || help
echo "$1" | grep -qi "^help\|-h" && help

log() {
	echo "$*" >&2
}

##   env
##     Print environment.
cmd_env() {
	test "$envread" = "yes" && return 0
	envread=yes

	if test -z "$__version"; then
		# Build a *correct* semantic version from date and time 
		__version=$(date +%Y.%_m.%_d+%H.%M | tr -d ' ')
	fi
	test -n "$__dest" || __dest=$dir

	if test "$cmd" = "env"; then
		opts="version|dest"
		set | grep -E "^(__($opts))="
		exit 0
	fi
}
##   dynamic [--version=] [--dest=]
##     Build with dynamic linking
cmd_dynamic() {
	test -e "$__dest" -a ! -d "$__dest" && \
		die "Exist but is not a directory [$__dest]"
	mkdir -p $__dest
	cd $dir
	local dst=$(readlink -f $__dest)
	go build -o $dst -ldflags "-X main.version=$__version" ./cmd/...
	strip $__dest/if-inject
}
##   static [--version=] [--dest=]
##     Build with static linking
cmd_static() {
	test -e "$__dest" -a ! -d "$__dest" && \
		die "Exist but is not a directory [$__dest]"
	mkdir -p $__dest
	local dst=$(readlink -f $__dest)
	cd $dir
	CGO_ENABLED=0 GOOS=linux go build -o $dst \
		-ldflags "-extldflags '-static' -X main.version=$__version" ./cmd/...
	strip $__dest/if-inject
}

##
# Get the command
cmd=$1
shift
grep -q "^cmd_$cmd()" $0 $hook || die "Invalid command [$cmd]"

while echo "$1" | grep -q '^--'; do
	if echo $1 | grep -q =; then
		o=$(echo "$1" | cut -d= -f1 | sed -e 's,-,_,g')
		v=$(echo "$1" | cut -d= -f2-)
		eval "$o=\"$v\""
	else
		if test "$1" = "--"; then
			shift
			break
		fi
		o=$(echo "$1" | sed -e 's,-,_,g')
		eval "$o=yes"
	fi
	shift
done
unset o v
long_opts=`set | grep '^__' | cut -d= -f1`

# Execute command
trap "die Interrupted" INT TERM
cmd_env
cmd_$cmd "$@"
status=$?
rm -rf $tmp
exit $status
