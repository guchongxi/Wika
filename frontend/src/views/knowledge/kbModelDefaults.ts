export type KBModelType = 'KnowledgeQA' | 'Embedding' | 'Rerank' | 'VLLM' | 'ASR'

export interface KBModelLike {
  id?: string
  type?: KBModelType
  is_default?: boolean
}

export interface KBModelConfigLike {
  llmModelId?: string
  embeddingModelId?: string
}

export type MissingKBModelKey = 'embedding' | 'llm'

export function pickPreferredKBModelId(models: KBModelLike[], type: KBModelType): string {
  const candidates = models.filter((model) => model.type === type && !!model.id)
  return candidates.find((model) => model.is_default)?.id || candidates[0]?.id || ''
}

export function getMissingExplicitKBModelKeys(input: {
  models: KBModelLike[]
  config: KBModelConfigLike
  needsEmbedding: boolean
}): MissingKBModelKey[] {
  const missing: MissingKBModelKey[] = []
  const hasModelType = (type: KBModelType) => input.models.some((model) => model.type === type && !!model.id)

  if (input.needsEmbedding && !input.config.embeddingModelId && hasModelType('Embedding')) {
    missing.push('embedding')
  }
  if (!input.config.llmModelId && hasModelType('KnowledgeQA')) {
    missing.push('llm')
  }

  return missing
}
