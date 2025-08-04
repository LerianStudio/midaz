import { SelectField } from '@/components/form'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { Paper } from '@/components/ui/paper'
import { SelectItem } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { getLocaleCode } from '@/lib/intl/get-locale-code'
import { useLocale } from '@/lib/intl/use-locale'
import languages from '@/lib/languages'
import React from 'react'
import { useForm } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { z } from 'zod'

const _formSchema = z.object({
  locale: z.string().min(1)
})

type FormSchema = z.infer<typeof _formSchema>

const initialValues = {
  locale: ''
}

export const SystemTabContent = () => {
  const intl = useIntl()
  const { locale, setLocale } = useLocale()

  const form = useForm<FormSchema>({
    defaultValues: { ...initialValues, locale }
  })

  const handleSubmit = (values: FormSchema) => {
    setLocale(values.locale)
    form.reset(values)
  }

  const handleCancel = () => form.reset()

  return (
    <React.Fragment>
      <div className="grid grid-cols-3">
        <Paper className="col-span-2 mb-6 flex flex-col">
          <p className="text-shadcn-400 p-6 text-sm font-medium">
            {intl.formatMessage({
              id: 'settings.system.paper.description',
              defaultMessage: 'Adjust system preferences.'
            })}
          </p>
          <Separator orientation="horizontal" />
          <Form {...form}>
            <div className="grid grid-cols-2 p-6">
              <SelectField
                label={intl.formatMessage({
                  id: 'settings.system.language',
                  defaultMessage: 'Language'
                })}
                name="locale"
                description={intl.formatMessage({
                  id: 'settings.system.locale.description',
                  defaultMessage:
                    'Select the language you would like to use on Midaz.'
                })}
                control={form.control}
              >
                {languages.map((language) => (
                  <SelectItem
                    key={language.locale}
                    value={getLocaleCode(language.locale)}
                  >
                    {language.name}
                    {' - '}
                    {language.locale.toLocaleUpperCase()}
                  </SelectItem>
                ))}
              </SelectField>
            </div>

            <PageFooter open={form.formState.isDirty}>
              <PageFooterSection>
                <Button variant="outline" onClick={handleCancel}>
                  {intl.formatMessage({
                    id: 'common.cancel',
                    defaultMessage: 'Cancel'
                  })}
                </Button>
              </PageFooterSection>
              <PageFooterSection>
                <Button onClick={form.handleSubmit(handleSubmit)}>
                  {intl.formatMessage({
                    id: 'common.save',
                    defaultMessage: 'Save'
                  })}
                </Button>
              </PageFooterSection>
            </PageFooter>
          </Form>
        </Paper>
      </div>
    </React.Fragment>
  )
}
