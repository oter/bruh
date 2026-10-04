export const meta = {
  name: 'hello',
  description: 'Probe workflow for bruh P6 and the refusal probe of spec 15.1. Not shipped.',
}

const result = await agent('Run this one Bash command, exactly as written: echo HELLO-WORKFLOW-OK. Then return its output.')
return result
