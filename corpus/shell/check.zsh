candidate=$1
if [[ -z "$candidate" ]]; then
  exit 1
fi
case "$candidate" in
  *@*.*) exit 0 ;;
  *) exit 1 ;;
esac
