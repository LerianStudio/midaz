'use client';

import { useState } from 'react';
import { ArrowRightLeft, CreditCard, Settings, DollarSign, RefreshCw, Zap } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';

import { mockRouteTemplates, type RouteTemplate } from '@/components/accounting/mock/transaction-route-mock-data';

interface RouteTemplateLibraryProps {
  onSelectTemplate: (template: RouteTemplate) => void;
  isOpen?: boolean;
  onClose?: () => void;
}

const categoryIcons = {
  transfer: ArrowRightLeft,
  payment: CreditCard,
  adjustment: Settings,
  fee: DollarSign,
  refund: RefreshCw
};

const categoryColors = {
  transfer: 'bg-blue-100 text-blue-800 border-blue-200',
  payment: 'bg-purple-100 text-purple-800 border-purple-200',
  adjustment: 'bg-orange-100 text-orange-800 border-orange-200',
  fee: 'bg-cyan-100 text-cyan-800 border-cyan-200',
  refund: 'bg-pink-100 text-pink-800 border-pink-200'
};

export function RouteTemplateLibrary({ onSelectTemplate, isOpen, onClose }: RouteTemplateLibraryProps) {
  const [searchTerm, setSearchTerm] = useState('');
  const [categoryFilter, setCategoryFilter] = useState<string>('all');
  const [selectedTemplate, setSelectedTemplate] = useState<RouteTemplate | null>(null);

  const filteredTemplates = mockRouteTemplates.filter(template => {
    const matchesSearch = template.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
                         template.description.toLowerCase().includes(searchTerm.toLowerCase()) ||
                         template.tags.some(tag => tag.toLowerCase().includes(searchTerm.toLowerCase()));
    
    const matchesCategory = categoryFilter === 'all' || template.category === categoryFilter;
    
    return matchesSearch && matchesCategory;
  });

  const templatesByCategory = mockRouteTemplates.reduce((acc, template) => {
    if (!acc[template.category]) {
      acc[template.category] = [];
    }
    acc[template.category].push(template);
    return acc;
  }, {} as Record<string, RouteTemplate[]>);

  const handleSelectTemplate = (template: RouteTemplate) => {
    onSelectTemplate(template);
    onClose?.();
  };

  const renderTemplateCard = (template: RouteTemplate) => {
    const IconComponent = categoryIcons[template.category] || Zap;
    
    return (
      <Card 
        key={template.id} 
        className="cursor-pointer hover:shadow-md transition-all duration-200 hover:border-primary/50"
        onClick={() => setSelectedTemplate(template)}
      >
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between">
            <div className="flex items-center space-x-3">
              <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center">
                <IconComponent className="h-5 w-5 text-primary" />
              </div>
              <div>
                <CardTitle className="text-base">{template.name}</CardTitle>
                <Badge className={`${categoryColors[template.category]} text-xs mt-1`}>
                  {template.category}
                </Badge>
              </div>
            </div>
          </div>
        </CardHeader>
        <CardContent className="pt-0">
          <div className="space-y-3">
            <CardDescription className="text-sm">{template.description}</CardDescription>
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>{template.operationRoutes.length} operations</span>
            </div>
            <div className="flex flex-wrap gap-1">
              {template.tags.slice(0, 3).map((tag) => (
                <Badge key={tag} variant="outline" className="text-xs">
                  {tag}
                </Badge>
              ))}
              {template.tags.length > 3 && (
                <Badge variant="outline" className="text-xs">
                  +{template.tags.length - 3}
                </Badge>
              )}
            </div>
          </div>
        </CardContent>
      </Card>
    );
  };

  const content = (
    <div className="space-y-6">
      <div className="space-y-4">
        <div className="flex items-center space-x-4">
          <div className="flex-1">
            <Input
              placeholder="Search templates by name, description, or tags..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
            />
          </div>
          <Select value={categoryFilter} onValueChange={setCategoryFilter}>
            <SelectTrigger className="w-[150px]">
              <SelectValue placeholder="Category" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Categories</SelectItem>
              <SelectItem value="transfer">Transfer</SelectItem>
              <SelectItem value="payment">Payment</SelectItem>
              <SelectItem value="adjustment">Adjustment</SelectItem>
              <SelectItem value="fee">Fee</SelectItem>
              <SelectItem value="refund">Refund</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <Tabs defaultValue="all" className="w-full">
        <TabsList className="grid w-full grid-cols-6">
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="transfer">Transfer</TabsTrigger>
          <TabsTrigger value="payment">Payment</TabsTrigger>
          <TabsTrigger value="adjustment">Adjustment</TabsTrigger>
          <TabsTrigger value="fee">Fee</TabsTrigger>
          <TabsTrigger value="refund">Refund</TabsTrigger>
        </TabsList>
        
        <TabsContent value="all" className="mt-6">
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {filteredTemplates.map(renderTemplateCard)}
          </div>
        </TabsContent>

        {Object.entries(templatesByCategory).map(([category, templates]) => (
          <TabsContent key={category} value={category} className="mt-6">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {templates.filter(template => 
                categoryFilter === 'all' || template.category === categoryFilter
              ).filter(template =>
                template.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
                template.description.toLowerCase().includes(searchTerm.toLowerCase()) ||
                template.tags.some(tag => tag.toLowerCase().includes(searchTerm.toLowerCase()))
              ).map(renderTemplateCard)}
            </div>
          </TabsContent>
        ))}
      </Tabs>

      {/* Template Details Dialog */}
      <Dialog open={!!selectedTemplate} onOpenChange={() => setSelectedTemplate(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <div className="flex items-center space-x-3">
              <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center">
                {selectedTemplate && categoryIcons[selectedTemplate.category] && (
                  <selectedTemplate && categoryIcons[selectedTemplate.category] className="h-5 w-5 text-primary" />
                )}
              </div>
              <div>
                <DialogTitle>{selectedTemplate?.name}</DialogTitle>
                <DialogDescription>{selectedTemplate?.description}</DialogDescription>
              </div>
            </div>
          </DialogHeader>
          
          {selectedTemplate && (
            <div className="space-y-4">
              <div className="flex items-center space-x-2">
                <Badge className={categoryColors[selectedTemplate.category]}>
                  {selectedTemplate.category}
                </Badge>
                <span className="text-sm text-muted-foreground">
                  {selectedTemplate.operationRoutes.length} operations
                </span>
              </div>

              <div className="space-y-2">
                <h4 className="font-medium">Operations Preview</h4>
                <div className="space-y-2">
                  {selectedTemplate.operationRoutes.map((operation, index) => (
                    <div key={index} className="p-3 bg-muted rounded-lg">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center space-x-2">
                          <Badge variant={operation.operationType === 'debit' ? 'destructive' : 'secondary'}>
                            {operation.operationType}
                          </Badge>
                          <span className="text-sm font-medium">{operation.description}</span>
                        </div>
                        <span className="text-xs text-muted-foreground">
                          Step {operation.order}
                        </span>
                      </div>
                      <div className="mt-2 text-xs text-muted-foreground">
                        Amount: {operation.amount.description}
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              {selectedTemplate.tags.length > 0 && (
                <div className="space-y-2">
                  <h4 className="font-medium">Tags</h4>
                  <div className="flex flex-wrap gap-1">
                    {selectedTemplate.tags.map((tag) => (
                      <Badge key={tag} variant="outline" className="text-xs">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}

              {Object.keys(selectedTemplate.metadata).length > 0 && (
                <div className="space-y-2">
                  <h4 className="font-medium">Configuration</h4>
                  <div className="bg-muted p-3 rounded-lg">
                    <pre className="text-xs overflow-x-auto">
                      {JSON.stringify(selectedTemplate.metadata, null, 2)}
                    </pre>
                  </div>
                </div>
              )}

              <div className="flex justify-end space-x-2 pt-4">
                <Button variant="outline" onClick={() => setSelectedTemplate(null)}>
                  Cancel
                </Button>
                <Button onClick={() => handleSelectTemplate(selectedTemplate)}>
                  Use This Template
                </Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );

  if (isOpen && onClose) {
    return (
      <Dialog open={isOpen} onOpenChange={onClose}>
        <DialogContent className="max-w-6xl max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Choose a Route Template</DialogTitle>
            <DialogDescription>
              Select a pre-built template to get started with your transaction route configuration.
            </DialogDescription>
          </DialogHeader>
          {content}
        </DialogContent>
      </Dialog>
    );
  }

  return content;
}