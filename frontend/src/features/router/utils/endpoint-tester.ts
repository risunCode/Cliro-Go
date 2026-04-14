export interface EndpointPreset {
  id: string
  label: string
  method: 'GET' | 'POST'
  path: string
  defaultBody: string
}

export interface TesterRenderedBlock {
  kind: 'thinking' | 'text'
  content: string
}

export interface TesterStructuredResponse {
  thinking: string
  message: string
}

const ENVIRONMENT_DETAILS_BLOCK = /<environment_details>[\s\S]*?<\/environment_details>/gi

const sanitizeVisibleResponseText = (value: string): string => {
  if (!value) {
    return ''
  }
  return value.replace(ENVIRONMENT_DETAILS_BLOCK, '').trim()
}

export const ENDPOINT_PRESETS: EndpointPreset[] = [
  { id: 'health', label: 'GET /health', method: 'GET', path: '/health', defaultBody: '' },
  { id: 'models', label: 'GET /v1/models', method: 'GET', path: '/v1/models', defaultBody: '' },
  { id: 'stats', label: 'GET /v1/stats', method: 'GET', path: '/v1/stats', defaultBody: '' },
  {
    id: 'responses',
    label: 'POST /v1/responses (Codex CLI)',
    method: 'POST',
    path: '/v1/responses',
    defaultBody: JSON.stringify(
      {
        model: 'gpt-5.3-codex',
 reasoning: { effort: 'medium' },
        input: 'Say hello from CLIro responses API.',
        stream: true
      },
      null,
      2
    )
  },
  {
    id: 'chat-completions',
    label: 'POST /v1/chat/completions (OpenAI/compatible)',
    method: 'POST',
    path: '/v1/chat/completions',
    defaultBody: JSON.stringify(
      {
        model: 'gpt-5.3-codex',
     reasoning: { effort: 'medium' },
        messages: [{ role: 'user', content: 'Say hello from CLIro.' }],
        stream: true
      },
      null,
      2
    )
  },
    {
 id: 'completions',
    label: 'POST /v1/completions (OpenAI/compatible)',
    method: 'POST',
    path: '/v1/completions',
    defaultBody: JSON.stringify(
      {
        model: 'gpt-5.3-codex',
        prompt: 'Write one sentence about local proxy routing.',
      stream: true
      },
      null,
      2
    )
  },
  {
    id: 'messages',
    label: 'POST /v1/messages (Anthropic/compatible)',
    method: 'POST',
    path: '/v1/messages',
    defaultBody: JSON.stringify(
      {
        model: 'claude-haiku-4.5',
        thinking: { type: 'enabled', budget_tokens: 2000 },
        max_tokens: 256,
        stream: true,
        messages: [{ role: 'user', content: 'Say hello from CLIro Anthropic-compatible endpoint.' }]
      },
      null,
      2
    )
  }
]

export const getEndpointPreset = (endpointID: string): EndpointPreset => {
  return ENDPOINT_PRESETS.find((endpoint) => endpoint.id === endpointID) || ENDPOINT_PRESETS[0]
}

export const getEndpointRequestBody = (endpointID: string): string => {
  const endpoint = getEndpointPreset(endpointID)
  return endpoint.method === 'POST' ? endpoint.defaultBody : ''
}

export const buildEndpointTarget = (baseURL: string, routePath: string): string => {
  const trimmedBase = baseURL.trim().replace(/\/+$/, '')
  const normalizedPath = routePath.startsWith('/') ? routePath : `/${routePath}`

  if (/\/v1$/i.test(trimmedBase) && /^\/v1(\/|$)/i.test(normalizedPath)) {
    return `${trimmedBase.slice(0, -3)}${normalizedPath}`
  }

  return `${trimmedBase}${normalizedPath}`
}

export const buildTesterStructuredResponse = (payload: string): TesterStructuredResponse | null => {
  const trimmed = payload.trim()
  if (!trimmed) {
    return null
  }

  try {
    const parsed = JSON.parse(trimmed) as Record<string, unknown>
    return extractStructuredResponseFromJSON(parsed)
  } catch {
    return extractStructuredResponseFromSSE(trimmed)
  }
}

export const extractStructuredResponseFromJSON = (payload: Record<string, unknown>): TesterStructuredResponse | null => {
  const blocks = extractMessageBlocks(payload)
  if (blocks.length > 0) {
    const thinking = sanitizeVisibleResponseText(blocks.filter((block) => block.kind === 'thinking').map((block) => block.content).join('\n\n'))
    const message = sanitizeVisibleResponseText(blocks.filter((block) => block.kind === 'text').map((block) => block.content).join('\n\n'))
    if (!thinking && !message) {
      return null
    }
    return { thinking, message }
  }

  const openAIStructured = extractStructuredResponseFromOpenAIJSON(payload)
  if (openAIStructured) {
    return openAIStructured
  }

  const response = payload.response
  if (response && typeof response === 'object') {
    return extractStructuredResponseFromJSON(response as Record<string, unknown>)
  }
  return null
}

export const extractStructuredResponseFromSSE = (payload: string): TesterStructuredResponse | null => {
  const sections = payload.split(/\r?\n\r?\n/)
  let thinking = ''
  let message = ''

  for (const section of sections) {
    const lines = section.split(/\r?\n/)
    let eventName = ''
    const dataLines: string[] = []
    for (const line of lines) {
      if (line.startsWith('event:')) {
        eventName = line.slice(6).trim()
      } else if (line.startsWith('data:')) {
        dataLines.push(line.slice(5).replace(/^ /, ''))
      }
    }
    if (dataLines.length === 0) {
      continue
    }

    const rawData = dataLines.join('\n')
    if (rawData === '[DONE]') {
      continue
    }
    try {
      const parsed = JSON.parse(rawData) as Record<string, unknown>
      if (!eventName) {
        const structured = extractStructuredResponseFromOpenAIJSON(parsed)
        if (structured) {
          thinking += structured.thinking
          message += structured.message
        }
        continue
      }
      switch (eventName) {
        case 'content_block_delta': {
          const delta = parsed.delta
          if (delta && typeof delta === 'object') {
            const deltaRecord = delta as Record<string, unknown>
            const deltaType = String(deltaRecord.type || '').trim()
            if (deltaType === 'thinking_delta') {
              thinking += String(deltaRecord.thinking || '')
            }
            if (deltaType === 'text_delta') {
              message += String(deltaRecord.text || '')
            }
          }
          break
        }
        case 'response.output_text.delta':
          thinking += extractOpenAIReasoning(parsed)
          message += String(parsed.delta || '')
          break
        case 'response.output_text.done':
          thinking += extractOpenAIReasoning(parsed)
          if (!message) {
            message = String(parsed.text || '')
          }
          break
        case 'response.completed': {
          const response = parsed.response
          if (response && typeof response === 'object') {
            const responseRecord = response as Record<string, unknown>
            if (!thinking) {
              thinking = extractOpenAIReasoning(responseRecord)
            }
            if (!message) {
              message = String(responseRecord.output_text || '')
            }
          }
          break
        }
        case 'message_start': {
          const messageRecord = parsed.message
          if (messageRecord && typeof messageRecord === 'object') {
            const structured = extractStructuredResponseFromJSON(messageRecord as Record<string, unknown>)
            if (structured) {
              if (!thinking) thinking = structured.thinking
              if (!message) message = structured.message
            }
          }
          break
        }
        default:
          break
      }
    } catch {
      continue
    }
  }

  const normalizedThinking = sanitizeVisibleResponseText(thinking)
  const normalizedMessage = sanitizeVisibleResponseText(message)
  if (!normalizedThinking && !normalizedMessage) {
    return null
  }
  return { thinking: normalizedThinking, message: normalizedMessage }
}

export const extractStructuredResponseFromOpenAIJSON = (payload: Record<string, unknown>): TesterStructuredResponse | null => {
  const choices = payload.choices
  if (!Array.isArray(choices)) {
    return null
  }

  let thinking = ''
  let message = ''

  for (const choice of choices) {
    if (!choice || typeof choice !== 'object') {
      continue
    }
    const choiceRecord = choice as Record<string, unknown>

    const messageRecord = choiceRecord.message
    if (messageRecord && typeof messageRecord === 'object') {
      thinking += extractOpenAIReasoning(messageRecord as Record<string, unknown>)
      const content = extractOpenAIMessageContent(messageRecord as Record<string, unknown>)
      if (content) {
        message += content
      }
    }

    const deltaRecord = choiceRecord.delta
    if (deltaRecord && typeof deltaRecord === 'object') {
      const delta = deltaRecord as Record<string, unknown>
      if (typeof delta.content === 'string') {
        message += delta.content
      }
      thinking += extractOpenAIReasoning(delta)
    }

    thinking += extractOpenAIReasoning(choiceRecord)

    if (typeof choiceRecord.text === 'string') {
      message += choiceRecord.text
    }
  }

  const normalizedThinking = sanitizeVisibleResponseText(thinking)
  const normalizedMessage = sanitizeVisibleResponseText(message)
  if (!normalizedThinking && !normalizedMessage) {
    return null
  }
  return { thinking: normalizedThinking, message: normalizedMessage }
}

export const extractOpenAIReasoning = (payload: Record<string, unknown>): string => {
  if (typeof payload.reasoning_content === 'string') {
    return payload.reasoning_content
  }
  if (typeof payload.reasoning === 'string') {
    return payload.reasoning
  }
  return ''
}

export const extractOpenAIMessageContent = (payload: Record<string, unknown>): string => {
  const content = payload.content
  if (typeof content === 'string') {
    return sanitizeVisibleResponseText(content)
  }
  if (!Array.isArray(content)) {
    return ''
  }

  const parts: string[] = []
  for (const item of content) {
    if (!item || typeof item !== 'object') {
      continue
    }
    const record = item as Record<string, unknown>
    if (typeof record.text === 'string' && record.text.trim()) {
      parts.push(record.text)
    }
  }
  return sanitizeVisibleResponseText(parts.join(''))
}

export const extractMessageBlocks = (payload: Record<string, unknown>): TesterRenderedBlock[] => {
  const content = payload.content
  if (!Array.isArray(content)) {
    return []
  }

  const blocks: TesterRenderedBlock[] = []
  for (const item of content) {
    if (!item || typeof item !== 'object') {
      continue
    }
    const record = item as Record<string, unknown>
    const type = String(record.type || '').trim()
    if (type === 'thinking') {
      const thinking = sanitizeVisibleResponseText(String(record.thinking || ''))
      if (thinking) {
        blocks.push({ kind: 'thinking', content: thinking })
      }
      continue
    }
    if (type === 'text') {
      const text = sanitizeVisibleResponseText(String(record.text || ''))
      if (text) {
        blocks.push({ kind: 'text', content: text })
      }
    }
  }
  return blocks
}
