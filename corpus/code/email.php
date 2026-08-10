<?php

function looks_like_email(string $value): bool
{
    $cleaned = trim($value);
    return str_contains($cleaned, '@') && str_contains($cleaned, '.');
}

function sum_values(array $values): int
{
    $total = 0;
    foreach ($values as $value) {
        $total += $value;
    }
    return $total;
}
