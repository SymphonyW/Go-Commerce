import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import EmptyState from '../components/EmptyState';
import LoadingState from '../components/LoadingState';
import PageHeader from '../components/PageHeader';
import ProductCard from '../components/ProductCard';
import SectionCard from '../components/SectionCard';
import { CATEGORY_OPTIONS } from '../utils/display';
import { productAPI } from '../services/api';

const DEFAULT_PAGE = 1;
const DEFAULT_PAGE_SIZE = 10;
const DEFAULT_SORT = 'created_at:desc';
const SORT_VALUES = ['created_at:desc', 'price_cents:asc', 'price_cents:desc', 'stock:asc', 'stock:desc'];

const readPositiveInt = (value, fallback) => {
  const parsed = Number.parseInt(value || '', 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
};

const Products = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const page = readPositiveInt(searchParams.get('page'), DEFAULT_PAGE);
  const pageSize = readPositiveInt(searchParams.get('page_size'), DEFAULT_PAGE_SIZE);
  const keyword = searchParams.get('keyword') || '';
  const category = searchParams.get('category') || '';
  const sortBy = searchParams.get('sort_by') || 'created_at';
  const order = searchParams.get('order') || 'desc';
  const requestedSortValue = `${sortBy}:${order}`;
  const sortValue = SORT_VALUES.includes(requestedSortValue) ? requestedSortValue : DEFAULT_SORT;

  const [products, setProducts] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [keywordInput, setKeywordInput] = useState(keyword);
  const [categoryInput, setCategoryInput] = useState(category);

  useEffect(() => {
    setKeywordInput(keyword);
    setCategoryInput(category);
  }, [keyword, category]);

  useEffect(() => {
    const fetchProducts = async () => {
      setLoading(true);
      try {
        const data = await productAPI.listProducts({
          page,
          page_size: pageSize,
          category: category || undefined,
          keyword: keyword || undefined,
          sort_by: sortBy,
          order,
        });
        setProducts(data.products || []);
        setTotal(data.total || 0);
      } catch (error) {
        console.error('Failed to fetch products:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchProducts();
  }, [page, pageSize, category, keyword, sortBy, order]);

  const totalPages = useMemo(() => Math.max(1, Math.ceil(total / pageSize)), [total, pageSize]);
  const hasFilters = Boolean(keyword || category);

  const updateQuery = (updates) => {
    const next = new URLSearchParams(searchParams);
    Object.entries(updates).forEach(([key, value]) => {
      if (value === '' || value === undefined || value === null) {
        next.delete(key);
      } else {
        next.set(key, String(value));
      }
    });
    setSearchParams(next);
  };

  const handleSubmit = (event) => {
    event.preventDefault();
    updateQuery({
      page: 1,
      keyword: keywordInput.trim(),
      category: categoryInput.trim(),
    });
  };

  const handleCategoryChip = (nextCategory) => {
    setCategoryInput(nextCategory);
    updateQuery({
      page: 1,
      category: nextCategory,
    });
  };

  const handleSortChange = (event) => {
    const [nextSortBy, nextOrder] = event.target.value.split(':');
    updateQuery({
      page: 1,
      sort_by: nextSortBy,
      order: nextOrder,
    });
  };

  const handleReset = () => {
    setKeywordInput('');
    setCategoryInput('');
    updateQuery({
      page: 1,
      keyword: '',
      category: '',
    });
  };

  const handlePageChange = (nextPage) => {
    if (nextPage < 1 || nextPage > totalPages) return;
    updateQuery({ page: nextPage });
  };

  return (
    <div className="products-container">
      <PageHeader
        eyebrow="Catalog"
        title="商品列表"
        subtitle="支持关键词、分类和排序组合筛选，适合直接展示分页与卡片布局。"
        meta={`共 ${total} 件`}
      />

      <SectionCard className="filter-panel" title="筛选商品" subtitle="先收窄范围，再继续浏览。">
        <form className="products-toolbar" onSubmit={handleSubmit}>
          <input
            type="search"
            value={keywordInput}
            onChange={(event) => setKeywordInput(event.target.value)}
            placeholder="搜索名称或描述"
            aria-label="搜索商品"
          />
          <input
            type="text"
            value={categoryInput}
            onChange={(event) => setCategoryInput(event.target.value)}
            placeholder="按分类筛选"
            aria-label="商品分类"
          />
          <select value={sortValue} onChange={handleSortChange} aria-label="商品排序">
            <option value="created_at:desc">最新上架</option>
            <option value="price_cents:asc">价格从低到高</option>
            <option value="price_cents:desc">价格从高到低</option>
            <option value="stock:desc">库存从高到低</option>
            <option value="stock:asc">库存从低到高</option>
          </select>
          <div className="filter-actions">
            <button type="submit" className="btn btn-primary">
              应用筛选
            </button>
            {hasFilters && (
              <button type="button" className="btn btn-secondary" onClick={handleReset}>
                重置
              </button>
            )}
          </div>
        </form>

        <div className="category-chips" aria-label="快捷分类">
          <button
            type="button"
            className={`category-chip ${!category ? 'active' : ''}`.trim()}
            onClick={() => handleCategoryChip('')}
          >
            全部
          </button>
          {CATEGORY_OPTIONS.map((option) => (
            <button
              key={option}
              type="button"
              className={`category-chip ${category === option ? 'active' : ''}`.trim()}
              onClick={() => handleCategoryChip(option)}
            >
              {option}
            </button>
          ))}
        </div>

        {hasFilters && (
          <div className="active-filters" aria-label="当前筛选">
            {keyword && <span>关键词：{keyword}</span>}
            {category && <span>分类：{category}</span>}
          </div>
        )}
      </SectionCard>

      {loading ? (
        <LoadingState label="正在加载商品..." />
      ) : products.length === 0 ? (
        <EmptyState
          title="没有找到符合条件的商品"
          description="换个关键词，或重置筛选后再试一次。"
          action={
            hasFilters && (
              <button type="button" className="btn btn-secondary btn-sm" onClick={handleReset}>
                重置筛选
              </button>
            )
          }
        />
      ) : (
        <>
          <div className="products-grid">
            {products.map((product) => (
              <ProductCard key={product.id} product={product} />
            ))}
          </div>

          <div className="pagination" aria-label="商品分页">
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => handlePageChange(page - 1)}
              disabled={page <= 1}
            >
              上一页
            </button>
            <span>
              第 {page} / {totalPages} 页
            </span>
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => handlePageChange(page + 1)}
              disabled={page >= totalPages}
            >
              下一页
            </button>
          </div>
        </>
      )}
    </div>
  );
};

export default Products;
