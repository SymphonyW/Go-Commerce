import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import EmptyState from '../components/EmptyState';
import LoadingState from '../components/LoadingState';
import MerchantConsoleNav from '../components/MerchantConsoleNav';
import PageHeader from '../components/PageHeader';
import SectionCard from '../components/SectionCard';
import { merchantAPI } from '../services/api';
import { centsToInputValue, formatMoney, parseMoneyToCents } from '../utils/display';

const emptyForm = {
  name: '',
  description: '',
  price: '',
  stock: '',
  category: '',
  image_url: '',
};

const MerchantProducts = () => {
  const navigate = useNavigate();
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const explicitMerchantId = searchParams.get('merchant_id') || id || undefined;
  const role = localStorage.getItem('role');
  const [products, setProducts] = useState([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [formData, setFormData] = useState(emptyForm);
  const [editingProductId, setEditingProductId] = useState(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const pageSize = 6;

  useEffect(() => {
    const token = localStorage.getItem('token');
    if (!token) {
      navigate('/login');
      return;
    }
    if (role !== 'merchant' && role !== 'admin') {
      navigate('/');
    }
  }, [navigate, role]);

  const params = useMemo(
    () => (explicitMerchantId ? { merchant_id: explicitMerchantId } : {}),
    [explicitMerchantId],
  );

  const loadProducts = useCallback(async (targetPage = page) => {
    try {
      setLoading(true);
      setError('');
      const data = await merchantAPI.listConsoleProducts({
        ...params,
        page: targetPage,
        page_size: pageSize,
      });
      setProducts(data.products || []);
      setTotal(data.total || 0);
    } catch (loadError) {
      setError(loadError.response?.data?.error || '获取商品列表失败');
    } finally {
      setLoading(false);
    }
  }, [page, params]);

  useEffect(() => {
    loadProducts(page);
  }, [loadProducts, page]);

  const handleChange = (event) => {
    const { name, value } = event.target;
    setFormData((prev) => ({ ...prev, [name]: value }));
  };

  const validateForm = () => {
    if (!formData.name.trim()) return '请输入商品名称';
    if (!formData.category.trim()) return '请输入商品分类';
    if (parseMoneyToCents(formData.price) === null) return '价格必须是大于等于 0 的金额，最多两位小数';
    if (formData.stock === '' || Number(formData.stock) < 0) return '库存必须大于等于 0';
    return '';
  };

  const handleSubmit = async (event) => {
    event.preventDefault();
    const validationError = validateForm();
    if (validationError) {
      setError(validationError);
      return;
    }

    const priceCents = parseMoneyToCents(formData.price);
    const payload = {
      name: formData.name.trim(),
      description: formData.description.trim(),
      category: formData.category.trim(),
      image_url: formData.image_url.trim(),
      price_cents: priceCents,
      stock: Number(formData.stock),
    };

    try {
      setSubmitting(true);
      setError('');
      setSuccess('');
      if (editingProductId) {
        await merchantAPI.updateConsoleProduct(editingProductId, payload, params);
        setSuccess('商品已更新');
      } else {
        await merchantAPI.createConsoleProduct(payload, params);
        setSuccess('商品已创建');
      }
      setFormData(emptyForm);
      setEditingProductId(null);
      await loadProducts(page);
    } catch (submitError) {
      setError(submitError.response?.data?.error || '保存商品失败');
    } finally {
      setSubmitting(false);
    }
  };

  const startEditing = (product) => {
    setEditingProductId(product.id);
    setFormData({
      name: product.name || '',
      description: product.description || '',
      price: centsToInputValue(product.price_cents),
      stock: product.stock ?? '',
      category: product.category || '',
      image_url: product.image_url || '',
    });
    setSuccess('');
    setError('');
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const resetForm = () => {
    setEditingProductId(null);
    setFormData(emptyForm);
    setSuccess('');
    setError('');
  };

  const handleDelete = async (product) => {
    if (!window.confirm(`确认删除商品“${product.name}”吗？`)) return;

    try {
      setSubmitting(true);
      setError('');
      setSuccess('');
      await merchantAPI.deleteConsoleProduct(product.id, params);
      setSuccess('商品已删除');
      const nextPage = products.length === 1 && page > 1 ? page - 1 : page;
      setPage(nextPage);
      await loadProducts(nextPage);
    } catch (deleteError) {
      setError(deleteError.response?.data?.error || '删除商品失败');
    } finally {
      setSubmitting(false);
    }
  };

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="merchant-console">
      <MerchantConsoleNav />

      <PageHeader
        eyebrow="商品管理"
        title="管理店铺商品"
        subtitle="在同一页完成新增、编辑、库存维护与列表查看。"
        meta={`${total} 件`}
        actions={
          editingProductId && (
            <button type="button" className="btn btn-secondary btn-sm" onClick={resetForm}>
              新增商品
            </button>
          )
        }
      />

      {error && <div className="error-message">{error}</div>}
      {success && <div className="success-message">{success}</div>}

      <div className="merchant-products-layout">
        <SectionCard
          title={editingProductId ? '编辑商品' : '新增商品'}
          subtitle={editingProductId ? '调整商品信息并保存。' : '补充完整字段，前端展示会更自然。'}
        >
          <form id="merchant-product-form" className="merchant-form" onSubmit={handleSubmit}>
            <div className="form-row">
              <div className="form-group">
                <label htmlFor="name">商品名称</label>
                <input id="name" name="name" value={formData.name} onChange={handleChange} required />
              </div>
              <div className="form-group">
                <label htmlFor="category">分类</label>
                <input id="category" name="category" value={formData.category} onChange={handleChange} required />
              </div>
            </div>
            <div className="form-row">
              <div className="form-group">
                <label htmlFor="price">价格</label>
                <input id="price" name="price" type="number" min="0" step="0.01" value={formData.price} onChange={handleChange} required />
              </div>
              <div className="form-group">
                <label htmlFor="stock">库存</label>
                <input id="stock" name="stock" type="number" min="0" value={formData.stock} onChange={handleChange} required />
              </div>
            </div>
            <div className="form-group">
              <label htmlFor="description">描述</label>
              <textarea id="description" name="description" value={formData.description} onChange={handleChange} />
            </div>
            <div className="form-group">
              <label htmlFor="image_url">图片 URL</label>
              <input id="image_url" name="image_url" value={formData.image_url} onChange={handleChange} />
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={submitting}>
                {submitting ? '保存中...' : editingProductId ? '保存修改' : '创建商品'}
              </button>
              {editingProductId && (
                <button type="button" className="btn btn-secondary" onClick={resetForm}>
                  取消编辑
                </button>
              )}
            </div>
          </form>
        </SectionCard>

        <SectionCard title="商品列表" subtitle={`共 ${total} 件商品，支持分页查看。`}>
          {loading ? (
            <LoadingState label="正在加载商品..." />
          ) : products.length === 0 ? (
            <EmptyState compact title="暂无商品" description="先创建第一件商品，店铺就会真正开始运转。" />
          ) : (
            <>
              <div className="merchant-table product-table">
                <div className="merchant-table-head">
                  <span>商品</span>
                  <span>分类</span>
                  <span>价格</span>
                  <span>库存</span>
                  <span>操作</span>
                </div>
                {products.map((product) => (
                  <div key={product.id} className="merchant-table-row">
                    <span>
                      <strong>{product.name}</strong>
                      <small>{product.description || '暂无描述'}</small>
                    </span>
                    <span>{product.category}</span>
                    <span>{formatMoney(product.price_cents)}</span>
                    <span>{product.stock}</span>
                    <span className="inline-actions">
                      <button type="button" className="btn btn-secondary btn-sm" onClick={() => startEditing(product)}>
                        编辑
                      </button>
                      <button type="button" className="btn btn-danger btn-sm" onClick={() => handleDelete(product)} disabled={submitting}>
                        删除
                      </button>
                    </span>
                  </div>
                ))}
              </div>

              <div className="pagination">
                <button type="button" className="btn btn-secondary btn-sm" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>
                  上一页
                </button>
                <span>
                  第 {page} / {totalPages} 页
                </span>
                <button type="button" className="btn btn-secondary btn-sm" disabled={page >= totalPages} onClick={() => setPage((value) => value + 1)}>
                  下一页
                </button>
              </div>
            </>
          )}
        </SectionCard>
      </div>
    </div>
  );
};

export default MerchantProducts;
