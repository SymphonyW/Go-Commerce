import { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { productAPI } from '../services/api';

const DEFAULT_PAGE = 1;
const DEFAULT_PAGE_SIZE = 10;
const DEFAULT_SORT = 'created_at:desc';

// 处理图片URL，将维基百科页面URL转换为实际图片文件URL
const processImageUrl = (url) => {
  if (!url) return 'https://via.placeholder.com/200';

  if (url.includes('wikipedia.org/wiki/File:')) {
    const fileName = url.split('/').pop();
    return `https://upload.wikimedia.org/wikipedia/commons/thumb/${fileName.charAt(0)}/${fileName.charAt(0) + fileName.charAt(1)}/${fileName}/200px-${fileName}`;
  }

  return url;
};

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
  const allowedSortValues = ['created_at:desc', 'price:asc', 'price:desc', 'stock:asc', 'stock:desc'];
  const sortValue = allowedSortValues.includes(requestedSortValue) ? requestedSortValue : DEFAULT_SORT;

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

  const handleSortChange = (event) => {
    const [nextSortBy, nextOrder] = event.target.value.split(':');
    updateQuery({
      page: 1,
      sort_by: nextSortBy,
      order: nextOrder,
    });
  };

  const handlePageChange = (nextPage) => {
    if (nextPage < 1 || nextPage > totalPages) {
      return;
    }
    updateQuery({ page: nextPage });
  };

  return (
    <div className="products-container">
      <div className="products-header">
        <div>
          <h1>商品列表</h1>
          <p className="products-summary">共 {total} 件商品</p>
        </div>
      </div>

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
        <select value={sortValue || DEFAULT_SORT} onChange={handleSortChange} aria-label="商品排序">
          <option value="created_at:desc">最新上架</option>
          <option value="price:asc">价格从低到高</option>
          <option value="price:desc">价格从高到低</option>
          <option value="stock:desc">库存从高到低</option>
          <option value="stock:asc">库存从低到高</option>
        </select>
        <button type="submit" className="btn">
          筛选
        </button>
      </form>

      {loading ? (
        <div className="loading">加载中...</div>
      ) : products.length === 0 ? (
        <div className="empty-state">没有找到符合条件的商品</div>
      ) : (
        <>
          <div className="products-grid">
            {products.map((product) => (
              <div key={product.id} className="product-card">
                <img
                  src={processImageUrl(product.image_url)}
                  alt={product.name}
                  className="product-image"
                />
                <h3>{product.name}</h3>
                <p className="product-price">¥{product.price}</p>
                <p className="product-stock">库存: {product.stock}</p>
                <Link to={`/products/${product.id}`} className="btn btn-sm">
                  查看详情
                </Link>
              </div>
            ))}
          </div>

          <div className="pagination">
            <button
              type="button"
              className="btn btn-sm"
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
              className="btn btn-sm"
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

