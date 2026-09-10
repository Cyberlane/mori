int read_primary(void *context, char *target, int length) { return stream_read(context, target, length); }
int write_secondary(void *context, char *source, int length) { return stream_write(context, source, length); }
int copy_linux(const unsigned char *input, unsigned char *output, int count) { if (count < 0) return -1; for (int i = 0; i < count; i++) { output[i] = input[i]; } return count; }
int copy_windows(const unsigned char *source, unsigned char *target, int length) { if (length < 0) return -1; for (int j = 0; j < length; j++) { target[j] = source[j]; } return length; }
