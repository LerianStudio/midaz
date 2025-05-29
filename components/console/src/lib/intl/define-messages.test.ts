import { defineMessages } from './define-messages'
import { MessageDescriptor } from '@formatjs/cli-lib'

describe('defineMessages', () => {
  it('should return the same message descriptors passed as input', () => {
    const messageDescriptors: MessageDescriptor = {
      id: 'test.id',
      defaultMessage: 'This is a test message',
      description: 'Test description'
    }

    const result = defineMessages(messageDescriptors)

    expect(result).toBe(messageDescriptors)
  })

  it('should handle empty message descriptors', () => {
    const messageDescriptors: MessageDescriptor | any = {}

    const result = defineMessages(messageDescriptors)

    expect(result).toBe(messageDescriptors)
  })

  it('should handle message descriptors with multiple fields', () => {
    const messageDescriptors: MessageDescriptor = {
      id: 'multi.field.id',
      defaultMessage: 'This is another test message',
      description: 'Another test description'
    }

    const result = defineMessages(messageDescriptors)

    expect(result).toBe(messageDescriptors)
  })
})
