// Orca plugin of i2a (inbox2agent). i2a calls the command "notify" through
// the Orca runtime RPC plugins.invokeCommand with { title, body }. The text
// comes from cards that an agent wrote after it read untrusted messages, so
// the plugin accepts only strings, removes control characters and cuts them.
const MAX_TITLE = 200
const MAX_BODY = 1000

function clean(value, max) {
  if (typeof value !== 'string') return ''
  const text = value.replace(/[\u0000-\u001f\u007f-\u009f]/g, ' ').replace(/\s+/g, ' ').trim()
  return text.length > max ? text.slice(0, max - 3) + '...' : text
}

export default function activate(orca) {
  orca.commands.register('notify', async (args) => {
    const title = clean(args?.title, MAX_TITLE)
    if (title === '') throw new Error('i2a: the title is empty')
    const body = clean(args?.body, MAX_BODY)
    await orca.host.call('notifications.show', body === '' ? { title } : { title, body })
    return { shown: true }
  })
}
