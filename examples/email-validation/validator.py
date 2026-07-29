def looks_like_email(value):
    cleaned = value.strip()
    return "@" in cleaned and "." in cleaned
