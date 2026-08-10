function looks_like_email(string $value): bool {
  $cleaned = Str\trim($value);
  return Str\contains($cleaned, '@') && Str\contains($cleaned, '.');
}
