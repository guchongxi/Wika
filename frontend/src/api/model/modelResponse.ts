type ModelListItem = {
  type?: string
}

type WrappedModelListResponse<T> = {
  success?: boolean
  data?: T[]
}

export function normalizeModelListResponse<T extends ModelListItem>(
  response: unknown,
  type?: string,
): T[] {
  const list = Array.isArray(response)
    ? response
    : isWrappedModelListResponse<T>(response)
      ? response.data
      : []

  return type ? list.filter((item) => item.type === type) : list
}

function isWrappedModelListResponse<T>(response: unknown): response is WrappedModelListResponse<T> {
  if (response == null || typeof response !== 'object') return false
  const candidate = response as WrappedModelListResponse<T>
  return candidate.success === true && Array.isArray(candidate.data)
}
