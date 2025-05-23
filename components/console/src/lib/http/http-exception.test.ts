import { HttpException } from './http-exception'
import { HttpStatus } from './http-status'

describe('HttpException', () => {
  it('should create an instance with a message and default status', () => {
    const message = 'An error occurred'
    const exception = new HttpException(message)

    expect(exception.message).toBe(message)
    expect(exception.getStatus()).toBe(HttpStatus.INTERNAL_SERVER_ERROR)
  })

  it('should create an instance with a message and custom status', () => {
    const message = 'Not Found'
    const status = HttpStatus.NOT_FOUND
    const exception = new HttpException(message, status)

    expect(exception.message).toBe(message)
    expect(exception.getStatus()).toBe(status)
  })

  it('should return the response object with the message', () => {
    const message = 'An error occurred'
    const exception = new HttpException(message)

    expect(exception.getResponse()).toEqual({ message })
  })
})
