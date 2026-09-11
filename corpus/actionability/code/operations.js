function addItem(items, value) { items.push(value); return items.length; }
function removeItem(items, value) { items.splice(value, 1); return items.length; }
function easeHorizontal(start, end, progress) { const distance = end - start; const eased = progress * progress; return start + distance * eased; }
function easeVertical(top, bottom, elapsed) { const distance = bottom - top; const eased = elapsed * elapsed; return top + distance * eased; }
