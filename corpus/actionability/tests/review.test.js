// Original MIT fixtures. Similar scaffolding does not imply shared test intent.
function suiteAccounts() {
  describe('accounts', () => { it('rejects expired tokens', () => expect(validateToken('old')).toBe(false)); });
}
function suiteImages() {
  describe('images', () => { it('keeps dimensions', () => expect(resizeImage(200)).toBe(100)); });
}
function assertAccountResponse(response) {
  if (response.status !== 200) throw new Error('request failed');
  const body = response.json();
  if (!body.id) throw new Error('missing identity');
  return body;
}
function assertProjectResponse(result) {
  if (result.status !== 200) throw new Error('request failed');
  const payload = result.json();
  if (!payload.id) throw new Error('missing identity');
  return payload;
}
