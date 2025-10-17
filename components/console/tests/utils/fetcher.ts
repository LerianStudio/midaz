export async function request(url: string, options?: RequestInit) {
  const response = await fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers
    }
  })

  if (!response.ok) {
    const errorText = await response.text()
    throw new Error(
      `HTTP error! status: ${response.status}, message: ${errorText}`
    )
  }

  if (response?.headers?.get('content-type')?.includes('text/plain')) {
    return await response.text()
  }

  return await response.json()
}

export async function getRequest(url: string, options?: RequestInit) {
  return await request(url, {
    method: 'GET',
    ...options
  })
}

export async function postRequest(
  url: string,
  body: any,
  options?: RequestInit
) {
  return await request(url, {
    method: 'POST',
    body: JSON.stringify(body),
    ...options
  })
}

export async function patchRequest(
  url: string,
  body: any,
  options?: RequestInit
) {
  return await fetch(url, {
    method: 'PATCH',
    body: JSON.stringify(body),
    ...options
  })
}

export async function deleteRequest(url: string, options?: RequestInit) {
  return await request(url, {
    method: 'DELETE',
    ...options
  })
}

export async function postFormDataRequest(
  url: string,
  formData: FormData,
  options?: RequestInit
) {
  const response = await fetch(url, {
    method: 'POST',
    body: formData,
    ...options,
    headers: {
      ...options?.headers
    }
  })

  return await response.json()
}
