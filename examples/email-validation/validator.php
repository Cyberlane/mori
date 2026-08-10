<?php

function looks_like_email(string $value): bool
{
    $cleaned = trim($value);
    return str_contains($cleaned, '@') && str_contains($cleaned, '.');
}
