/**
 * Function copied from react-intl to define messages.
 *
 * This is a hook for babel-plugin-react-intl to extract messages.
 * It's needed since react-intl is a client side code unusable on the server side.
 * @param messageDescriptors - The message descriptors to be defined.
 * @returns
 */
export function defineMessages(messageDescriptors: any) {
  // This simply returns what's passed-in because it's meant to be a hook for
  // babel-plugin-react-intl.
  return messageDescriptors
}
