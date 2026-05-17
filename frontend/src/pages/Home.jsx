import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import EmptyState from '../components/EmptyState';
import LoadingState from '../components/LoadingState';
import ProductCard from '../components/ProductCard';
import SectionCard from '../components/SectionCard';
import { CATEGORY_OPTIONS } from '../utils/display';
import { productAPI } from '../services/api';

const highlights = [
  {
    title: '精选商品',
    description: '围绕真实分类组织商品，让浏览、筛选和详情展示都更完整。',
  },
  {
    title: '稳定交易体验',
    description: '从购物车到订单状态流转，完整呈现电商核心链路。',
  },
  {
    title: '商家高效经营',
    description: '轻量后台承接商品维护、订单查看与店铺管理。',
  },
];

const Home = () => {
  const [products, setProducts] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchProducts = async () => {
      try {
        const data = await productAPI.listProducts({ page: 1, page_size: 20 });
        setProducts(data.products || []);
      } catch (error) {
        console.error('Failed to fetch products:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchProducts();
  }, []);

  const featuredProducts = useMemo(() => {
    const diverseSelection = CATEGORY_OPTIONS.flatMap((category) =>
      products.filter((product) => product.category === category).slice(0, 2),
    );
    const remaining = products.filter((product) => !diverseSelection.some((item) => item.id === product.id));
    return [...diverseSelection, ...remaining].slice(0, 8);
  }, [products]);
  const categoryStats = useMemo(
    () =>
      CATEGORY_OPTIONS.map((category) => ({
        name: category,
        count: products.filter((product) => product.category === category).length,
      })),
    [products],
  );

  return (
    <div className="home">
      <section className="hero">
        <div className="hero-content">
          <p className="eyebrow">Go Commerce</p>
          <h1>把完整交易链路，做成愿意继续浏览的电商体验。</h1>
          <p>
            从精选商品到商家后台，Go Commerce 让商品、订单与履约过程在同一个演示系统里自然衔接。
          </p>
          <div className="hero-actions">
            <Link to="/products" className="btn btn-primary">
              浏览商品
            </Link>
            <Link to="/merchants" className="btn btn-secondary">
              探索商家
            </Link>
          </div>
        </div>
      </section>

      <section className="feature-grid" aria-label="平台亮点">
        {highlights.map((item) => (
          <article key={item.title} className="feature-item">
            <strong>{item.title}</strong>
            <p>{item.description}</p>
          </article>
        ))}
      </section>

      <SectionCard
        title="热门推荐"
        subtitle="优先展示最近上架的商品，让首页在导入演示数据后立即有内容可看。"
        action={
          <Link to="/products" className="btn btn-secondary btn-sm">
            查看全部
          </Link>
        }
      >
        {loading ? (
          <LoadingState label="正在加载精选商品..." />
        ) : featuredProducts.length === 0 ? (
          <EmptyState
            title="还没有可展示的商品"
            description="导入演示数据后，这里会出现完整的商品陈列。"
            action={
              <Link to="/products" className="btn btn-primary btn-sm">
                前往商品页
              </Link>
            }
          />
        ) : (
          <div className="products-grid">
            {featuredProducts.map((product) => (
              <ProductCard key={product.id} product={product} />
            ))}
          </div>
        )}
      </SectionCard>

      <SectionCard title="按分类探索" subtitle="用四条清晰的浏览路径，把演示数据的层次感完整展开。">
        <div className="category-grid">
          {categoryStats.map((category) => (
            <Link
              key={category.name}
              to={`/products?category=${encodeURIComponent(category.name)}`}
              className="category-card"
            >
              <strong>{category.name}</strong>
              <p>{category.count > 0 ? `${category.count} 件商品可浏览` : '等待补充商品'}</p>
              <span>进入分类 →</span>
            </Link>
          ))}
        </div>
      </SectionCard>

      <section className="cta-banner">
        <div>
          <p className="eyebrow">Demo Ready</p>
          <h2>一套能真正撑起展示截图的电商前端。</h2>
          <p>导入演示数据后，首页、列表、详情、购物车与后台都会进入完整状态。</p>
        </div>
        <Link to="/products" className="btn btn-primary">
          开始浏览
        </Link>
      </section>
    </div>
  );
};

export default Home;
