import { RefObject, useState, useCallback } from 'react';
import { toast } from 'sonner';

export interface UseExportOptions {
  backgroundColor?: string;
  scale?: number;
  filenamePrefix?: string;
}

export interface UseExportResult {
  isExporting: boolean;
  exportAsImage: () => Promise<void>;
  exportAsPDF: () => Promise<void>;
}

export function useExport(
  elementRef: RefObject<HTMLElement>,
  options: UseExportOptions = {}
): UseExportResult {
  const {
    backgroundColor = '#0a0a0a',
    scale = 2,
    filenamePrefix = 'export',
  } = options;

  const [isExporting, setIsExporting] = useState(false);

  const exportAsImage = useCallback(async () => {
    if (!elementRef.current) return;
    
    setIsExporting(true);
    try {
      const html2canvas = (await import('html2canvas')).default;
      const canvas = await html2canvas(elementRef.current, {
        backgroundColor,
        scale,
      });
      
      const link = document.createElement('a');
      link.download = `${filenamePrefix}.png`;
      link.href = canvas.toDataURL('image/png');
      link.click();
      
      toast.success('Exported as image');
    } catch (error) {
      console.error('Export failed:', error);
      toast.error('Failed to export image');
    } finally {
      setIsExporting(false);
    }
  }, [elementRef, backgroundColor, scale, filenamePrefix]);

  const exportAsPDF = useCallback(async () => {
    if (!elementRef.current) return;
    
    setIsExporting(true);
    try {
      const html2canvas = (await import('html2canvas')).default;
      const { jsPDF } = await import('jspdf');
      
      const canvas = await html2canvas(elementRef.current, {
        backgroundColor,
        scale,
      });
      
      const imgData = canvas.toDataURL('image/png');
      const pdf = new jsPDF({
        orientation: canvas.width > canvas.height ? 'landscape' : 'portrait',
        unit: 'px',
        format: [canvas.width, canvas.height],
      });
      
      pdf.addImage(imgData, 'PNG', 0, 0, canvas.width, canvas.height);
      pdf.save(`${filenamePrefix}.pdf`);
      
      toast.success('Exported as PDF');
    } catch (error) {
      console.error('Export failed:', error);
      toast.error('Failed to export PDF');
    } finally {
      setIsExporting(false);
    }
  }, [elementRef, backgroundColor, scale, filenamePrefix]);

  return {
    isExporting,
    exportAsImage,
    exportAsPDF,
  };
}
