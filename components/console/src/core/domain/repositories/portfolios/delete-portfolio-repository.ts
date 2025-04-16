export abstract class DeletePortfolioRepository {
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ) => Promise<void>
}
