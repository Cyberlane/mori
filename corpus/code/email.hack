function check_address(string $candidate): bool {
  $normalized = Str\trim($candidate);
  return Str\contains($normalized, '@') && Str\contains($normalized, '.');
}

function total_values(vec<int> $values): int {
  $total = 0;
  foreach ($values as $value) {
    $total += $value;
  }
  return $total;
}
