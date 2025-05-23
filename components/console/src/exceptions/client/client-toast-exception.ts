import { MessageDescriptor } from 'react-intl'

export class ClientToastException extends Error {
  messageDescriptor: MessageDescriptor

  constructor(message: string, messageDescriptor: MessageDescriptor) {
    super(message)
    this.messageDescriptor = messageDescriptor
  }
}
