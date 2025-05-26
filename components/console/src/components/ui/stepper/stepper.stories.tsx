import { Meta, StoryObj } from '@storybook/react'
import { Button } from '../button'
import { HTMLAttributes } from 'react'
import { Stepper, StepperItem, StepperItemNumber, StepperItemText } from '.'
import { Paper } from '../paper'
import { useStepper } from '@/hooks/use-stepper'

const meta: Meta<HTMLAttributes<HTMLDivElement>> = {
  title: 'Primitives/Stepper',
  component: Stepper
}

export default meta

export const Primary: StoryObj<HTMLAttributes<HTMLDivElement>> = {
  render: (args) => (
    <div className="bg-zinc-100 p-6">
      <Stepper {...args}>
        <StepperItem complete>
          <StepperItemNumber>1</StepperItemNumber>
          <StepperItemText
            title="Details"
            description="Here you insert the details information"
          />
        </StepperItem>
        <StepperItem active>
          <StepperItemNumber>2</StepperItemNumber>
          <StepperItemText
            title="Address"
            description="Here you insert the address information"
          />
        </StepperItem>
        <StepperItem>
          <StepperItemNumber>3</StepperItemNumber>
          <StepperItemText
            title="Theme"
            description="Here you insert the theme information"
          />
        </StepperItem>
      </Stepper>
    </div>
  )
}

export const Controlled: StoryObj<HTMLAttributes<HTMLDivElement>> = {
  render: (args) => {
    const { step, handlePrevious, handleNext } = useStepper({ maxSteps: 3 })

    return (
      <div className="grid grid-cols-4 gap-10 bg-zinc-100 p-6">
        <Stepper {...args}>
          <StepperItem active={step === 0} complete={step > 0}>
            <StepperItemNumber>1</StepperItemNumber>
            <StepperItemText
              title="Details"
              description="Here you insert the details information"
            />
          </StepperItem>
          <StepperItem active={step === 1} complete={step > 1}>
            <StepperItemNumber>2</StepperItemNumber>
            <StepperItemText
              title="Address"
              description="Here you insert the address information"
            />
          </StepperItem>
          <StepperItem active={step === 2} complete={step > 2}>
            <StepperItemNumber>3</StepperItemNumber>
            <StepperItemText
              title="Theme"
              description="Here you insert the theme information"
            />
          </StepperItem>
        </Stepper>
        <Paper className="col-span-3 flex flex-row justify-center gap-4 p-6">
          <Button onClick={handlePrevious}>Back</Button>
          <Button onClick={handleNext}>Next</Button>
        </Paper>
      </div>
    )
  }
}
