export async function fetchApi(baseUrl: string, method: string, path: string, body?: unknown): Promise<unknown> {
  const url = `${baseUrl}${path}`;
  const options: RequestInit = {
    method,
    headers: { "Content-Type": "application/json" },
  };
  if (body !== undefined) {
    options.body = JSON.stringify(body);
  }
  const res = await fetch(url, options);
  const text = await res.text();
  if (!res.ok) {
    throw new Error(`API ${method} ${path} failed (${res.status}): ${text}`);
  }
  return text ? JSON.parse(text) : {};
}
