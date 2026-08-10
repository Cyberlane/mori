value=$1
if [ -z "$value" ]; then
  exit 1
fi
case "$value" in
  *@*.*) exit 0 ;;
  *) exit 1 ;;
esac
